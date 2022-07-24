package main

import (
	_ "./lib/go-sqlite3"

	"compress/gzip"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

var SETTINGS = struct {
	VERSION          string
	DATA_SOURCE_NAME string
	TOKEN            string
	GZIP             bool
}{
	VERSION:          "20220710",
	DATA_SOURCE_NAME: "./lnxmon.db",
	TOKEN:            "123456",
	GZIP:             true,
}

func Skip(err error) {
	if err != nil {
		log.Println(err)
		log.Println("skip error")
	}
}

func Throw(err error) {
	if err != nil {
		panic(err)
	}
}

func Catch() {
	var err interface{}
	err = recover()
	if err != nil {
		log.Println(err)
		log.Println(string(debug.Stack()))
	}
}

func Catch500(response http.ResponseWriter) {
	var err interface{}
	err = recover()
	if err != nil {
		log.Println(err)
		log.Println(string(debug.Stack()))
		Api(response, 500)
	}
}

func TimeTaken(started time.Time, action string) {
	var elapsed time.Duration
	elapsed = time.Since(started)
	log.Printf("%v took %v\n", action, elapsed)
}

func MakeHandler(next func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		defer Catch500(response)
		defer TimeTaken(time.Now(), request.URL.Path)

		log.Println("request.URL.Path:", request.URL.Path)

		if strings.HasPrefix(request.URL.Path, "/api/report_") {
			var token string
			token = request.Header.Get("token")

			if token != SETTINGS.TOKEN {
				Api(response, 401)
			} else {
				next(response, request)
			}
		} else {
			next(response, request)
		}
	}
}

type GzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

// > GzipResponseWriter.Write is ambiguous
// GzipResponseWriter embeded io.Writer and http.ResponseWriter, both of them have method Write(), without overriding, the selector will never know the method Write() belongs to which one when it is invoked, thus the compiler complains this error.
func (gzip_response GzipResponseWriter) Write(data []byte) (int, error) {
	// // > runtime: goroutine stack exceeds 1000000000-byte limit
	// // > fatal error: stack overflow
	// // because of recursive invocation
	// return gzip_response.Write(data)

	// With Gzip
	return gzip_response.Writer.Write(data)

	// // Without Gzip
	// return gzip_response.ResponseWriter.Write(data)
}

func MakeGzipHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		if SETTINGS.GZIP {
			if strings.Contains(request.Header.Get("Accept-Encoding"), "gzip") {
				var gzip_writer *gzip.Writer
				gzip_writer = gzip.NewWriter(response)
				defer gzip_writer.Close()

				var gzip_response GzipResponseWriter
				gzip_response = GzipResponseWriter{
					Writer:         gzip_writer,
					ResponseWriter: response,
				}

				// https://github.com/golang/go/issues/14975
				gzip_response.Header().Del("Content-Length")
				gzip_response.Header().Set("Content-Encoding", "gzip")

				next(gzip_response, request)
			} else {
				next(response, request)
			}
		} else {
			next(response, request)
		}
	}
}

func Api(response http.ResponseWriter, code int, args ...interface{}) {
	var err error

	var data map[string]interface{}
	data = map[string]interface{}{
		"code": code,
		"msg":  http.StatusText(code),
	}

	if len(args) == 0 {
	} else if len(args) == 1 {
		data["data"] = args[0]
	} else if len(args) == 2 {
		data["data"] = args[0]

		var key string
		var value interface{}

		for key, value = range args[1].(map[string]interface{}) {
			data[key] = value
		}
	} else {
	}

	var body []byte
	body, err = json.Marshal(data)
	Throw(err)

	log.Println("code:", code)

	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	// https://github.com/golang/go/blob/go1.17.8/src/net/http/server.go#L2058
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.WriteHeader(code)
	response.Write(body)
}

func HttpStatusOk(response http.ResponseWriter, request *http.Request) {
	response.WriteHeader(http.StatusOK)
}

// // If key is not present, FormValue returns the empty string.
// https://github.com/golang/go/blob/go1.17.8/src/net/http/request.go#L1344
func FormValueOf(request *http.Request, key string) string {
	if request.Form == nil {
		var defaultMaxMemory int64
		// 32 MiB
		defaultMaxMemory = 32 << 20
		request.ParseMultipartForm(defaultMaxMemory)
	}

	var value string
	value = "<nil>"

	var values []string
	values = request.Form[key]

	if len(values) > 0 {
		value = strings.TrimSpace(values[0])
	}

	return value
}

func IsSet(args ...string) bool {
	return !IsNotSet(args...)

}

// is_not_set = nil || null || None || undefined
// is_empty = "" || [] || {}
// is_blank = is_not_set + is_empty
//
// is_not_set = is_not_filled
// is_blank = is_not_present
func IsNotSet(args ...string) bool {
	var result bool

	var value string
	for _, value = range args {
		if value == "<nil>" || value == "undefined" {
			result = true
			break
		}
	}

	return result
}

func IsInt(args ...string) bool {
	return !IsNotInt(args...)
}

// If value doesn't exist, then continue.
// If value exists, then it must be an integer.
func IsNotInt(args ...string) bool {
	var err error

	var result bool

	var value string
	for _, value = range args {
		if value == "<nil>" || value == "undefined" {
			continue
		}

		_, err = strconv.ParseInt(value, 10, 64)
		if err != nil {
			result = true
			break
		}
	}

	return result
}

func ParseNil(value string) interface{} {
	if value == "<nil>" {
		return nil
	}
	return value
}

func SelectProjects(db *sql.DB) []map[string]interface{} {
	var err error

	var projects []map[string]interface{}

	{
		var query string
		query = "SELECT DISTINCT project FROM t_host ORDER BY project"

		var rows *sql.Rows
		rows, err = db.Query(query)
		defer rows.Close()
		Throw(err)

		for rows.Next() {
			var project string

			err = rows.Scan(&project)
			Throw(err)

			projects = append(
				projects,
				map[string]interface{}{
					"code": project,
					"name": strings.ToUpper(project),
				},
			)
		}
	}

	return projects
}

func SelectHosts(db *sql.DB, project string) []map[string]interface{} {
	var err error

	var query string
	query = `
		SELECT
			host.id,
			host.code,
			host.hostname,
			host.alias,
			host.ip,
			host.os_type,
			host.architecture,
			host.cpu_processors,
			host.mem_size,
			host.disk_size,
			host.uptime,
			host.heartbeat_time,
			host_metric.loadavg_1m,
			host_metric.loadavg_5m,
			host_metric.loadavg_15m,
			host_metric.cpu_used,
			host_metric.cpu_iowait,
			host_metric.mem_used,
			host_metric.swap_used,
			host_metric.disk_used,
			host_metric.inode_used,
			host_metric.users
		FROM t_host host
		JOIN t_host_metric_%s host_metric
		ON host.host_metric_id=host_metric.id
		WHERE host.project=?
		ORDER BY host.hostname
	`
	query = fmt.Sprintf(query, project)

	var rows *sql.Rows
	rows, err = db.Query(query, project)
	defer rows.Close()
	Throw(err)

	var hosts []map[string]interface{}
	hosts = make([]map[string]interface{}, 0)

	for rows.Next() {
		var id int64
		var code string
		var hostname string
		var alias sql.NullString
		var ip string
		var os_type string
		var architecture string
		var cpu_processors int64
		var mem_size int64
		var disk_size int64
		var uptime float64
		var heartbeat_time time.Time
		var loadavg_1m float64
		var loadavg_5m float64
		var loadavg_15m float64
		var cpu_used float64
		var cpu_iowait float64
		var mem_used float64
		var swap_used float64
		var disk_used float64
		var inode_used float64
		var users int64

		var alias2 string
		var ips []string
		var heartbeat_time2 string

		var loadavg string
		var cpu_usage int64
		var mem_usage int64
		var disk_usage int64

		var max_loadavg float64
		var max_cpu_usage int64
		var max_mem_usage int64
		var max_disk_usage int64

		var is_overload bool
		var is_overcpu bool
		var is_overmem bool
		var is_overdisk bool

		err = rows.Scan(
			&id,
			&code,
			&hostname,
			&alias,
			&ip,
			&os_type,
			&architecture,
			&cpu_processors,
			&mem_size,
			&disk_size,
			&uptime,
			&heartbeat_time,
			&loadavg_1m,
			&loadavg_5m,
			&loadavg_15m,
			&cpu_used,
			&cpu_iowait,
			&mem_used,
			&swap_used,
			&disk_used,
			&inode_used,
			&users,
		)
		Throw(err)

		if !alias.Valid {
			alias2 = alias.String
		}
		ips = strings.Split(ip, ",")
		heartbeat_time2 = heartbeat_time.Format("2006-01-02 15:04:05")

		{
			loadavg = fmt.Sprintf("%.2f, %.2f, %.2f", loadavg_1m, loadavg_5m, loadavg_15m)
			max_loadavg = math.Max(max_loadavg, loadavg_1m)
			max_loadavg = math.Max(max_loadavg, loadavg_5m)
			max_loadavg = math.Max(max_loadavg, loadavg_15m)
			if max_loadavg > float64(cpu_processors) {
				is_overload = true
			}
		}

		{
			cpu_usage = int64(math.Max(cpu_used, cpu_iowait))
			max_cpu_usage = cpu_usage
			if max_cpu_usage > 80 {
				is_overcpu = true
			}
		}

		{
			mem_usage = int64(math.Max(mem_used, swap_used))
			max_mem_usage = mem_usage
			if max_mem_usage > 80 {
				is_overmem = true
			}
		}

		{
			disk_usage = int64(math.Max(disk_used, inode_used))
			max_disk_usage = disk_usage
			// if max_disk_usage > 80 {
			if max_disk_usage > 85 {
				is_overdisk = true
			}
		}

		hosts = append(
			hosts,
			map[string]interface{}{
				"id":             id,
				"code":           code,
				"hostname":       hostname,
				"alias":          alias2,
				"ip":             ip,
				"ips":            ips,
				"os_type":        os_type,
				"architecture":   architecture,
				"cpu_processors": cpu_processors,
				"mem_size":       mem_size,
				"disk_size":      disk_size,
				"uptime":         uptime,
				"heartbeat_time": heartbeat_time2,
				"loadavg":        loadavg,
				"cpu_usage":      cpu_usage,
				"mem_usage":      mem_usage,
				"disk_usage":     disk_usage,
				"users":          users,
				"project":        project,
				"max_loadavg":    max_loadavg,
				"max_cpu_usage":  max_cpu_usage,
				"max_mem_usage":  max_mem_usage,
				"max_disk_usage": max_disk_usage,
				"is_overload":    is_overload,
				"is_overcpu":     is_overcpu,
				"is_overmem":     is_overmem,
				"is_overdisk":    is_overdisk,
			},
		)
	}

	return hosts
}

func SelectHost(db *sql.DB, id int64) map[string]interface{} {
	var err error

	log.Println("id:", id)

	var query string
	query = "SELECT code, project, cpu_processors FROM t_host WHERE id=?"

	var row *sql.Row
	row = db.QueryRow(query, id)

	var code string
	var project string
	var cpu_processors int64

	err = row.Scan(&code, &project, &cpu_processors)
	Throw(err)

	var host map[string]interface{}
	host = map[string]interface{}{
		"id":             id,
		"code":           code,
		"project":        project,
		"cpu_processors": cpu_processors,
	}

	return host
}

func SelectHostMetric(db *sql.DB, project string, code string, offset int64, limit int64) map[string]interface{} {
	var err error

	log.Println("project:", project)
	log.Println("code:", code)
	log.Println("offset:", offset)
	log.Println("limit:", limit)

	var now time.Time
	now = time.Now()

	var begin_time string
	begin_time = now.Add(-(time.Duration(offset) * time.Minute)).Format("2006-01-02 15:04:05")
	log.Println("begin_time:", begin_time)

	var end_time string
	if offset <= limit || limit == -1 {
		end_time = now.Format("2006-01-02 15:04:05")
	} else {
		end_time = now.Add(-(time.Duration(offset-limit) * time.Minute)).Format("2006-01-02 15:04:05")
	}
	log.Println("end_time:", end_time)

	var query string
	query = `
		SELECT
			loadavg_1m,
			loadavg_5m,
			loadavg_15m,
			cpu_used,
			cpu_iowait,
			mem_used,
			swap_used,
			disk_usage,
			disk_read_rate,
			disk_write_rate,
			nic_receive_rate,
			nic_transmit_rate,
			tcp_sockets_inuse,
			tcp_sockets_tw,
			users,
			heartbeat_time
		FROM t_host_metric_%s
		WHERE code=? AND heartbeat_time>=? AND heartbeat_time<=?
	`
	query = fmt.Sprintf(query, project)

	var rows *sql.Rows
	rows, err = db.Query(query, code, begin_time, end_time)
	defer rows.Close()
	Throw(err)

	var loadavg_array []map[string]interface{}
	var loadavg_1m_array []float64
	var loadavg_5m_array []float64
	var loadavg_15m_array []float64

	loadavg_array = make([]map[string]interface{}, 0)
	loadavg_1m_array = make([]float64, 0)
	loadavg_5m_array = make([]float64, 0)
	loadavg_15m_array = make([]float64, 0)

	var cpu_usage_array []map[string]interface{}
	var cpu_used_array []float64
	var cpu_iowait_array []float64

	cpu_usage_array = make([]map[string]interface{}, 0)
	cpu_used_array = make([]float64, 0)
	cpu_iowait_array = make([]float64, 0)

	var mem_usage_array []map[string]interface{}
	var mem_used_array []float64
	var swap_used_array []float64

	mem_usage_array = make([]map[string]interface{}, 0)
	mem_used_array = make([]float64, 0)
	swap_used_array = make([]float64, 0)

	var disk_usage_array []map[string]interface{}
	var disk_usage_map map[string][]float64

	disk_usage_array = make([]map[string]interface{}, 0)
	disk_usage_map = make(map[string][]float64)

	var disk_io_rate_array []map[string]interface{}
	var disk_read_rate_array []float64
	var disk_write_rate_array []float64

	disk_io_rate_array = make([]map[string]interface{}, 0)
	disk_read_rate_array = make([]float64, 0)
	disk_write_rate_array = make([]float64, 0)

	var nic_io_rate_array []map[string]interface{}
	var nic_receive_rate_array []float64
	var nic_transmit_rate_array []float64

	nic_io_rate_array = make([]map[string]interface{}, 0)
	nic_receive_rate_array = make([]float64, 0)
	nic_transmit_rate_array = make([]float64, 0)

	var tcp_sockets_array []map[string]interface{}
	var tcp_sockets_inuse_array []int64
	var tcp_sockets_tw_array []int64

	tcp_sockets_array = make([]map[string]interface{}, 0)
	tcp_sockets_inuse_array = make([]int64, 0)
	tcp_sockets_tw_array = make([]int64, 0)

	var misc_array []map[string]interface{}
	var users_array []float64

	misc_array = make([]map[string]interface{}, 0)
	users_array = make([]float64, 0)

	var heartbeat_time_array []string
	heartbeat_time_array = make([]string, 0)

	for rows.Next() {
		var loadavg_1m float64
		var loadavg_5m float64
		var loadavg_15m float64
		var cpu_used float64
		var cpu_iowait float64
		var mem_used float64
		var swap_used float64
		var disk_usage string
		var disk_read_rate float64
		var disk_write_rate float64
		var nic_receive_rate float64
		var nic_transmit_rate float64
		var tcp_sockets_inuse int64
		var tcp_sockets_tw int64
		var users string
		var heartbeat_time time.Time

		err = rows.Scan(
			&loadavg_1m,
			&loadavg_5m,
			&loadavg_15m,
			&cpu_used,
			&cpu_iowait,
			&mem_used,
			&swap_used,
			&disk_usage,
			&disk_read_rate,
			&disk_write_rate,
			&nic_receive_rate,
			&nic_transmit_rate,
			&tcp_sockets_inuse,
			&tcp_sockets_tw,
			&users,
			&heartbeat_time,
		)
		Throw(err)

		{
			loadavg_1m_array = append(loadavg_1m_array, loadavg_1m)
			loadavg_5m_array = append(loadavg_5m_array, loadavg_5m)
			loadavg_15m_array = append(loadavg_15m_array, loadavg_15m)
		}

		{
			cpu_used_array = append(cpu_used_array, cpu_used)
			cpu_iowait_array = append(cpu_iowait_array, cpu_iowait)
		}

		{
			mem_used_array = append(mem_used_array, mem_used)
			swap_used_array = append(swap_used_array, swap_used)
		}

		{
			var fields []string
			fields = strings.Split(disk_usage, ",")

			var field string
			for _, field = range fields {
				var fields2 []string
				fields2 = strings.Split(field, "_")

				var mount_point string
				var disk_total string
				var disk_used string
				var inode_used string

				mount_point = fields2[0]
				disk_total = fields2[1]
				disk_used = fields2[2]
				inode_used = fields2[3]

				var key_disk string
				var key_inode string
				var disk_used2 float64
				var inode_used2 float64

				key_disk = fmt.Sprintf("Disk Usage of %s (%sG)", mount_point, disk_total)
				key_inode = fmt.Sprintf("Inode Usage of %s (%sG)", mount_point, disk_total)
				disk_used2, err = strconv.ParseFloat(disk_used, 64)
				Skip(err)
				inode_used2, err = strconv.ParseFloat(inode_used, 64)
				Skip(err)

				disk_usage_map[key_disk] = append(disk_usage_map[key_disk], disk_used2)
				disk_usage_map[key_inode] = append(disk_usage_map[key_inode], inode_used2)
			}
		}

		{
			disk_read_rate_array = append(disk_read_rate_array, disk_read_rate)
			disk_write_rate_array = append(disk_write_rate_array, disk_write_rate)
		}

		{
			nic_receive_rate_array = append(nic_receive_rate_array, nic_receive_rate)
			nic_transmit_rate_array = append(nic_transmit_rate_array, nic_transmit_rate)
		}

		{
			tcp_sockets_inuse_array = append(tcp_sockets_inuse_array, tcp_sockets_inuse)
			tcp_sockets_tw_array = append(tcp_sockets_tw_array, tcp_sockets_tw)
		}

		{
			var users2 float64
			users2, err = strconv.ParseFloat(users, 64)
			Skip(err)
			users_array = append(users_array, users2)
		}

		{
			heartbeat_time_array = append(heartbeat_time_array, heartbeat_time.Format("2006-01-02 15:04:05"))
		}
	}

	var generate_series func(name string, data interface{}) map[string]interface{}
	generate_series = func(name string, data interface{}) map[string]interface{} {
		return map[string]interface{}{"name": name, "data": data}
	}

	loadavg_array = append(loadavg_array, generate_series("loadavg_1m", loadavg_1m_array))
	loadavg_array = append(loadavg_array, generate_series("loadavg_5m", loadavg_5m_array))
	loadavg_array = append(loadavg_array, generate_series("loadavg_15m", loadavg_15m_array))

	cpu_usage_array = append(cpu_usage_array, generate_series("cpu_usage", cpu_used_array))
	cpu_usage_array = append(cpu_usage_array, generate_series("cpu_iowait", cpu_iowait_array))

	mem_usage_array = append(mem_usage_array, generate_series("mem_usage", mem_used_array))
	mem_usage_array = append(mem_usage_array, generate_series("swap_usage", swap_used_array))

	var key string
	var value interface{}

	for key, value = range disk_usage_map {
		disk_usage_array = append(disk_usage_array, generate_series(key, value))
	}

	disk_io_rate_array = append(disk_io_rate_array, generate_series("read_rate", disk_read_rate_array))
	disk_io_rate_array = append(disk_io_rate_array, generate_series("write_rate", disk_write_rate_array))

	nic_io_rate_array = append(nic_io_rate_array, generate_series("reveive_rate", nic_receive_rate_array))
	nic_io_rate_array = append(nic_io_rate_array, generate_series("transmit_rate", nic_transmit_rate_array))

	tcp_sockets_array = append(tcp_sockets_array, generate_series("inuse", tcp_sockets_inuse_array))
	tcp_sockets_array = append(tcp_sockets_array, generate_series("tw", tcp_sockets_tw_array))

	misc_array = append(misc_array, generate_series("users", users_array))

	var host_metric map[string]interface{}
	host_metric = map[string]interface{}{
		"loadavg_array":        loadavg_array,
		"cpu_usage_array":      cpu_usage_array,
		"mem_usage_array":      mem_usage_array,
		"disk_usage_array":     disk_usage_array,
		"disk_io_rate_array":   disk_io_rate_array,
		"nic_io_rate_array":    nic_io_rate_array,
		"tcp_sockets_array":    tcp_sockets_array,
		"misc_array":           misc_array,
		"heartbeat_time_array": heartbeat_time_array,
	}

	return host_metric
}

func Index(response http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		Api(response, 404)
		return
	}

	var err error

	var project string
	var id string
	var offset string
	var limit string
	var mode string

	project = FormValueOf(request, "project")
	id = FormValueOf(request, "id")
	offset = FormValueOf(request, "offset")
	limit = FormValueOf(request, "limit")
	mode = FormValueOf(request, "mode")

	if IsNotInt(id, offset, limit) {
		Api(response, 400)
		return
	}

	if IsNotSet(project) {
		project = "default"
	} else {
		project = strings.ToLower(project)
	}

	var offset2 int64
	if IsNotSet(offset) {
		offset2 = 240
	} else {
		offset2, err = strconv.ParseInt(offset, 10, 64)
		Skip(err)
	}

	var limit2 int64
	if IsNotSet(limit) {
		// 60 * 24 * 31
		limit2 = 44640
	} else {
		limit2, err = strconv.ParseInt(limit, 10, 64)
		Skip(err)
		// if limit2 == -1 {
		// 	// 1,000,000,000
		// 	limit2 = 1000000000
		// }
	}

	if mode != "1" {
		mode = "0"
	}

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var projects []map[string]interface{}
	projects = SelectProjects(db)

	var host map[string]interface{}
	var hosts []map[string]interface{}

	var id2 int64
	if IsNotSet(id) {
		hosts = SelectHosts(db, project)

		if len(hosts) > 0 {
			host = hosts[0]
			id2 = host["id"].(int64)
		}
	} else {
		id2, err = strconv.ParseInt(id, 10, 64)
		Skip(err)

		host = SelectHost(db, id2)
		project = host["project"].(string)
		hosts = SelectHosts(db, project)
	}

	// > interface conversion: interface {} is nil, not string
	// host["code"] could be nil when there is no data
	var code string
	code = host["code"].(string)

	var host_metric map[string]interface{}
	host_metric = SelectHostMetric(db, project, code, offset2, limit2)

	var state map[string]interface{}
	state = map[string]interface{}{
		"offset": offset2,
		"mode":   mode,
	}

	var data struct {
		Projects   []map[string]interface{}
		Host       map[string]interface{}
		Hosts      []map[string]interface{}
		HostMetric map[string]interface{}
		State      map[string]interface{}
	}
	data.Projects = projects
	data.Host = host
	data.Hosts = hosts
	data.HostMetric = host_metric
	data.State = state

	var HTML string
	// Replace it when packaging static files into a single file
	HTML = ""

	var tpl *template.Template
	if HTML == "" {
		tpl, err = template.ParseFiles("template/index.html")
		Skip(err)
	} else {
		tpl, err = template.New("X").Parse(HTML)
		Skip(err)
	}
	tpl.Execute(response, data)
}

func ReportHost(response http.ResponseWriter, request *http.Request) {
	var err error

	var body []byte
	body, err = ioutil.ReadAll(request.Body)
	log.Println(string(body))
	Throw(err)

	var data map[string]interface{}
	json.Unmarshal(body, &data)

	if len(data) == 0 {
		Api(response, 400)
		return
	}

	var code string
	var hostname string
	var ip string
	var os_type string
	var architecture string
	var cpu_processors int64
	var mem_size int64
	var swap_size int64
	var disk_size int64
	var uptime float64
	var heartbeat_time string
	var project string
	var version string

	code = data["code"].(string)
	hostname = data["hostname"].(string)
	ip = data["ip"].(string)
	os_type = data["os_type"].(string)
	architecture = data["architecture"].(string)
	cpu_processors = int64(data["cpu_processors"].(float64))
	mem_size = int64(data["mem_size"].(float64))
	swap_size = int64(data["swap_size"].(float64))
	disk_size = int64(data["disk_size"].(float64))
	uptime = data["uptime"].(float64)
	heartbeat_time = data["heartbeat_time"].(string)
	project = data["project"].(string)
	version = data["version"].(string)

	project = strings.ToLower(project)

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var rows_affected int64

	{
		var query string
		query = `
			UPDATE t_host
			SET
				hostname=?, ip=?, os_type=?, architecture=?, cpu_processors=?,
				mem_size=?, swap_size=?, disk_size=?, uptime=?, heartbeat_time=?, version=?
			WHERE project=? AND code=?
		`

		var result sql.Result
		result, err = db.Exec(
			query,
			hostname, ip, os_type, architecture, cpu_processors,
			mem_size, swap_size, disk_size, uptime, heartbeat_time, version,
			project, code,
		)
		Throw(err)

		rows_affected, err = result.RowsAffected()
		log.Println("rows_affected:", rows_affected)
		Throw(err)
	}

	if rows_affected == 0 {
		var query string
		query = `
			INSERT INTO t_host (
				code, hostname, ip, os_type, architecture, cpu_processors,
				mem_size, swap_size, disk_size, uptime, heartbeat_time, project, version
			) VALUES (
				?,?,?,?,?,?,?,?,?,?,?,?,?
			)
		`

		_, err = db.Exec(
			query,
			code, hostname, ip, os_type, architecture, cpu_processors,
			mem_size, swap_size, disk_size, uptime, heartbeat_time, project, version,
		)
		Throw(err)
	}

	Api(response, 200)
}

func ReportHostMetric(response http.ResponseWriter, request *http.Request) {
	var err error

	var body []byte
	body, err = ioutil.ReadAll(request.Body)
	log.Println(string(body))
	Throw(err)

	var data map[string]interface{}
	json.Unmarshal(body, &data)

	if len(data) == 0 {
		Api(response, 400)
		return
	}

	var code string
	var hostname string
	var ip string
	var loadavg_1m float64
	var loadavg_5m float64
	var loadavg_15m float64
	var cpu_used float64
	var cpu_iowait float64
	var mem_used float64
	var swap_used float64
	var disk_usage string
	var disk_used float64
	var inode_used float64
	var disk_read_rate float64
	var disk_write_rate float64
	var disk_ios float64
	var nic_receive_rate float64
	var nic_receive_packets float64
	var nic_transmit_rate float64
	var nic_transmit_packets float64
	var tcp_sockets_inuse float64
	var tcp_sockets_tw float64
	var users float64
	var heartbeat_time string
	var project string

	code = data["code"].(string)
	hostname = data["hostname"].(string)
	ip = data["ip"].(string)
	loadavg_1m = data["loadavg_1m"].(float64)
	loadavg_5m = data["loadavg_5m"].(float64)
	loadavg_15m = data["loadavg_15m"].(float64)
	cpu_used = data["cpu_used"].(float64)
	cpu_iowait = data["cpu_iowait"].(float64)
	mem_used = data["mem_used"].(float64)
	swap_used = data["swap_used"].(float64)
	disk_usage = data["disk_usage"].(string)
	disk_used = data["disk_used"].(float64)
	inode_used = data["inode_used"].(float64)
	disk_read_rate = data["disk_read_rate"].(float64)
	disk_write_rate = data["disk_write_rate"].(float64)
	disk_ios = data["disk_ios"].(float64)
	nic_receive_rate = data["nic_receive_rate"].(float64)
	nic_receive_packets = data["nic_receive_packets"].(float64)
	nic_transmit_rate = data["nic_transmit_rate"].(float64)
	nic_transmit_packets = data["nic_transmit_packets"].(float64)
	tcp_sockets_inuse = data["tcp_sockets_inuse"].(float64)
	tcp_sockets_tw = data["tcp_sockets_tw"].(float64)
	users = data["users"].(float64)
	heartbeat_time = data["heartbeat_time"].(string)
	project = data["project"].(string)

	project = strings.ToLower(project)

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var tx *sql.Tx
	tx, err = db.Begin()
	defer tx.Rollback()
	Throw(err)

	var last_insert_id int64

	{
		var query string
		query = `
			INSERT INTO t_host_metric_%s (
				code,
				hostname,
				ip,
				loadavg_1m, loadavg_5m, loadavg_15m,
				cpu_used, cpu_iowait,
				mem_used, swap_used,
				disk_usage, disk_used, inode_used,
				disk_read_rate, disk_write_rate, disk_ios,
				nic_receive_rate, nic_receive_packets, nic_transmit_rate, nic_transmit_packets,
				tcp_sockets_inuse, tcp_sockets_tw,
				users,
				heartbeat_time
			) VALUES (
				?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?
			)
		`
		query = fmt.Sprintf(query, project)

		var stmt *sql.Stmt
		stmt, err = tx.Prepare(query)
		defer stmt.Close()
		if err != nil {
			Skip(err)
			log.Println(err.Error())
			if strings.Contains(err.Error(), "no such table") {
				CreateTableHostMetric(project)
			}
			stmt, err = tx.Prepare(query)
			Throw(err)
		}

		var result sql.Result
		result, err = stmt.Exec(
			code,
			hostname,
			ip,
			loadavg_1m, loadavg_5m, loadavg_15m,
			cpu_used, cpu_iowait,
			mem_used, swap_used,
			disk_usage, disk_used, inode_used,
			disk_read_rate, disk_write_rate, disk_ios,
			nic_receive_rate, nic_receive_packets, nic_transmit_rate, nic_transmit_packets,
			tcp_sockets_inuse, tcp_sockets_tw,
			users,
			heartbeat_time,
		)
		Throw(err)

		last_insert_id, err = result.LastInsertId()
		log.Println("last_insert_id:", last_insert_id)
		Throw(err)
	}

	{
		var query string
		query = "UPDATE t_host SET host_metric_id=?, heartbeat_time=? WHERE project=? AND code=?"
		_, err = tx.Exec(query, last_insert_id, heartbeat_time, project, code)
		Throw(err)
	}

	tx.Commit()
	log.Println("tx committed")

	Api(response, 200)
}

func GetProjects(response http.ResponseWriter, request *http.Request) {
	var err error

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var projects []map[string]interface{}
	projects = SelectProjects(db)

	Api(response, 200, projects)
}

func GetHosts(response http.ResponseWriter, request *http.Request) {
	var err error

	var project string
	project = FormValueOf(request, "project")

	if IsNotSet(project) {
		project = "default"
	} else {
		project = strings.ToLower(project)
	}

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var hosts []map[string]interface{}
	hosts = SelectHosts(db, project)

	Api(response, 200, hosts)
}

func GetHost(response http.ResponseWriter, request *http.Request) {
	var err error

	var id string
	id = FormValueOf(request, "id")

	if IsNotSet(id) || IsNotInt(id) {
		Api(response, 400)
		return
	}

	var id2 int64
	id2, err = strconv.ParseInt(id, 10, 64)
	Skip(err)

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var host map[string]interface{}
	host = SelectHost(db, id2)

	Api(response, 200, host)
}

func GetHostMetric(response http.ResponseWriter, request *http.Request) {
	var err error

	var id string
	var offset string
	var limit string

	id = FormValueOf(request, "id")
	offset = FormValueOf(request, "offset")
	limit = FormValueOf(request, "limit")

	if IsNotSet(id) || IsNotInt(id, offset, limit) {
		Api(response, 400)
		return
	}

	var id2 int64
	id2, err = strconv.ParseInt(id, 10, 64)
	Skip(err)

	var offset2 int64
	if IsNotSet(offset) {
		offset2 = 240
	} else {
		offset2, err = strconv.ParseInt(offset, 10, 64)
		Skip(err)
	}

	var limit2 int64
	if IsNotSet(limit) {
		// 60 * 24 * 31
		limit2 = 44640
	} else {
		limit2, err = strconv.ParseInt(limit, 10, 64)
		Skip(err)
		// if limit2 == -1 {
		// 	// 1,000,000,000
		// 	limit2 = 1000000000
		// }
	}

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var host map[string]interface{}
	host = SelectHost(db, id2)

	var host_metric map[string]interface{}
	host_metric = SelectHostMetric(db, host["project"].(string), host["code"].(string), offset2, limit2)

	Api(response, 200, host_metric)
}

func CreateTableHost() {
	var err error

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var query string
	query = "SELECT 1 FROM t_host"

	var rows *sql.Rows
	rows, err = db.Query(query)
	if rows != nil {
		defer rows.Close()
	}
	Skip(err)

	if rows == nil {
		var query2 string
		// query2 = `
		// 	CREATE TABLE t_host (
		// 		id                 INTEGER PRIMARY KEY AUTOINCREMENT,
		// 		code               VARCHAR(32)   NOT NULL,
		// 		hostname           VARCHAR(64)   DEFAULT NULL,
		// 		alias              VARCHAR(64)   DEFAULT NULL,
		// 		ip                 VARCHAR(100)  DEFAULT NULL,
		// 		os_type            VARCHAR(64)   DEFAULT NULL,
		// 		architecture       VARCHAR(64)   DEFAULT NULL,
		// 		cpu_processors     INTEGER       DEFAULT NULL,
		// 		mem_size           INTEGER       DEFAULT NULL,
		// 		swap_size          INTEGER       DEFAULT NULL,
		// 		disk_size          INTEGER       DEFAULT NULL,
		// 		uptime             DECIMAL(10,2) DEFAULT NULL,
		// 		heartbeat_time     DATETIME      DEFAULT NULL,
		// 		max_host_metric_id INTEGER       DEFAULT NULL,
		// 		project            VARCHAR(32)   NOT NULL,
		// 		version            VARCHAR(16)   DEFAULT NULL,
		// 		UNIQUE(project, code)
		// 	)
		// `
		query2 = `
			CREATE TABLE t_host (
				id                 INTEGER PRIMARY KEY AUTOINCREMENT,
				code               VARCHAR(32)   NOT NULL,
				hostname           VARCHAR(64)   NOT NULL,
				alias              VARCHAR(64)   DEFAULT NULL,
				ip                 VARCHAR(100)  NOT NULL,
				os_type            VARCHAR(64)   NOT NULL,
				architecture       VARCHAR(64)   NOT NULL,
				cpu_processors     INTEGER       NOT NULL,
				mem_size           INTEGER       NOT NULL,
				swap_size          INTEGER       NOT NULL,
				disk_size          INTEGER       NOT NULL,
				uptime             DECIMAL(10,2) NOT NULL,
				heartbeat_time     DATETIME      NOT NULL,
				host_metric_id     INTEGER       DEFAULT NULL,
				project            VARCHAR(32)   NOT NULL,
				version            VARCHAR(16)   NOT NULL,
				UNIQUE(project, code)
			)
		`

		_, err = db.Exec(query2)
		Throw(err)

		log.Println("created table t_host")
	}
}

func CreateTableHostMetric(project string) {
	var err error

	project = strings.ToLower(project)

	var db *sql.DB
	db, err = sql.Open("sqlite3", SETTINGS.DATA_SOURCE_NAME)
	defer db.Close()
	Throw(err)

	var query string
	query = "SELECT 1 FROM t_host_metric_%s"
	query = fmt.Sprintf(query, project)

	var rows *sql.Rows
	rows, err = db.Query(query)
	if rows != nil {
		defer rows.Close()
	}
	Skip(err)

	if rows == nil {
		{
			var query2 string
			// query2 = `
			// 	CREATE TABLE t_host_metric_%s (
			// 		id			   INTEGER PRIMARY KEY AUTOINCREMENT,
			// 		code           VARCHAR(32)  NOT NULL,
			// 		hostname       VARCHAR(64)  DEFAULT NULL,
			// 		ip             VARCHAR(100) DEFAULT NULL,
			// 		loadavg        VARCHAR(20)  DEFAULT NULL,
			// 		cpu_usage      VARCHAR(20)  DEFAULT NULL,
			// 		mem_usage      VARCHAR(20)  DEFAULT NULL,
			// 		disk_usage     VARCHAR(255) DEFAULT NULL,
			// 		disk_io_rate   VARCHAR(32)  DEFAULT NULL,
			// 		nic_io_rate    VARCHAR(32)  DEFAULT NULL,
			// 		tcp_sockets    VARCHAR(20)  DEFAULT NULL,
			// 		users          INTEGER      DEFAULT NULL,
			// 		heartbeat_time DATETIME     DEFAULT NULL
			// 	)
			// `
			query2 = `
				CREATE TABLE t_host_metric_%s (
					id                        INTEGER PRIMARY KEY AUTOINCREMENT,
					code                      VARCHAR(32)   NOT NULL,
					hostname                  VARCHAR(64)   NOT NULL,
					ip                        VARCHAR(100)  NOT NULL,
					loadavg_1m                DECIMAL(10,2) NOT NULL,
					loadavg_5m                DECIMAL(10,2) NOT NULL,
					loadavg_15m               DECIMAL(10,2) NOT NULL,
					cpu_used                  DECIMAL(10,2) NOT NULL,
					cpu_iowait                DECIMAL(10,2) NOT NULL,
					mem_used                  DECIMAL(10,2) NOT NULL,
					swap_used                 DECIMAL(10,2) NOT NULL,
					disk_usage                VARCHAR(255)  NOT NULL,
					disk_used                 DECIMAL(10,2) NOT NULL,
					inode_used                DECIMAL(10,2) NOT NULL,
					disk_read_rate            DECIMAL(10,2) NOT NULL,
					disk_write_rate           DECIMAL(10,2) NOT NULL,
					disk_ios                  INTEGER       NOT NULL,
					nic_receive_rate          DECIMAL(10,2) NOT NULL,
					nic_receive_packets       INTEGER       NOT NULL,
					nic_transmit_rate         DECIMAL(10,2) NOT NULL,
					nic_transmit_packets      INTEGER       NOT NULL,
					tcp_sockets_inuse         INTEGER       NOT NULL,
					tcp_sockets_tw            INTEGER       NOT NULL,
					users                     INTEGER       NOT NULL,
					heartbeat_time            DATETIME      NOT NULL
				)
			`
			query2 = fmt.Sprintf(query2, project)

			_, err = db.Exec(query2)
			Throw(err)
		}

		{
			var query2 string
			query2 = "CREATE INDEX idx__t_host_metric_%s__heartbeat_time ON t_host_metric_%s (heartbeat_time)"
			query2 = fmt.Sprintf(query2, project, project)
			_, err = db.Exec(query2)
			Throw(err)
		}

		log.Printf("created table t_host_metric_%v\n", project)
	}
}

func InitDb() {
	CreateTableHost()
	CreateTableHostMetric("DEFAULT")
}

func main() {
	defer Catch()

	var err error

	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var host string
	var port int
	var gzip bool

	flag.StringVar(&host, "host", "0.0.0.0", "Host")
	flag.IntVar(&port, "port", 1234, "Port")
	flag.BoolVar(&gzip, "gzip", true, "Gzip")

	flag.Parse()

	log.Printf("host: %v\n", host)
	log.Printf("port: %v\n", port)
	log.Printf("gzip: %v\n", gzip)

	var address string
	// :1234, 0.0.0.0:1234, 127.0.0.1:1234
	address = fmt.Sprintf("%s:%d", host, port)
	log.Printf("address: %v\n", address)

	SETTINGS.GZIP = gzip
	log.Printf("SETTINGS.GZIP: %v\n", gzip)

	log.Printf("SETTINGS: %+v\n", SETTINGS)

	InitDb()

	http.HandleFunc("/", MakeHandler(MakeGzipHandler(Index)))
	http.HandleFunc("/index", MakeHandler(MakeGzipHandler(Index)))
	http.HandleFunc("/favicon.ico", MakeHandler(HttpStatusOk))
	http.HandleFunc("/api/report_host", MakeHandler(ReportHost))
	http.HandleFunc("/api/report_host_metric", MakeHandler(ReportHostMetric))
	http.HandleFunc("/api/get_projects", MakeHandler(GetProjects))
	http.HandleFunc("/api/get_hosts", MakeHandler(GetHosts))
	http.HandleFunc("/api/get_host", MakeHandler(GetHost))
	http.HandleFunc("/api/get_host_metric", MakeHandler(MakeGzipHandler(GetHostMetric)))

	var fileServerHandler http.Handler
	fileServerHandler = http.FileServer(http.Dir("./static/"))
	http.Handle("/static/", http.StripPrefix("/static/", fileServerHandler))

	log.Printf("ListenAndServe: http://%v/\n", address)
	err = http.ListenAndServe(address, nil)
	Throw(err)
}
