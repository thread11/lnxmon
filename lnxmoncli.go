package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var SETTINGS = struct {
	VERSION string
	DEBUG   bool
	API     string
	PROJECT string
	TOKEN   string
}{
	VERSION: "20220710",
	DEBUG:   false,
	API:     "http://127.0.0.1:1234/api",
	PROJECT: "DEFAULT",
	TOKEN:   "123456",
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

func TimeTaken(started time.Time, action string) {
	var elapsed time.Duration
	elapsed = time.Since(started)
	log.Printf("%v took %v\n", action, elapsed)
}

func ExecCmd(command string) (string, error) {
	var err error

	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd = exec.Command("sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()

	err = cmd.Wait()

	var output string
	if err == nil {
		output = stdout.String()
	} else {
		output = stderr.String()
	}

	return output, err
}

func ExecCmdWithTimeout(command string, args ...time.Duration) (string, error) {
	var err error

	var duration time.Duration
	duration = 10
	if len(args) == 1 {
		duration = args[0]
	}

	var cmd *exec.Cmd
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd = exec.Command("sh", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	cmd.Start()

	var done chan error
	done = make(chan error)
	go func() { done <- cmd.Wait() }()

	var timeout <-chan time.Time
	timeout = time.After(duration * time.Second)

	select {
	case <-timeout:
		cmd.Process.Kill()
		return "", errors.New(fmt.Sprintf("command timed out after %d secs", duration))
	case err = <-done:
		var output string
		if err == nil {
			output = stdout.String()
		} else {
			output = stderr.String()
		}
		return output, err
	}
}

func HttpPost(api string, data []byte) int64 {
	defer Catch()
	defer TimeTaken(time.Now(), api)

	var err error

	var request *http.Request
	request, err = http.NewRequest("POST", api, bytes.NewReader(data))
	Throw(err)

	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("token", SETTINGS.TOKEN)

	var client *http.Client
	client = &http.Client{Timeout: 30 * time.Second}

	var response *http.Response
	response, err = client.Do(request)
	if response != nil {
		defer response.Body.Close()
	}
	Throw(err)

	var http_status_code int64
	if response != nil {
		log.Println("response status:", response.Status)
		log.Println("response headers:", response.Header)

		var body []byte
		body, err = ioutil.ReadAll(response.Body)
		log.Println("response body:", string(body))
		Throw(err)

		http_status_code = int64(response.StatusCode)
	}

	return http_status_code
}

func GetCode() string {
	var err error

	var hostname string
	hostname, err = os.Hostname()
	Throw(err)

	var md5sum [16]byte
	md5sum = md5.Sum([]byte(hostname))

	var code string
	code = fmt.Sprintf("%x", md5sum)

	return code
}

func GetHostname() string {
	var err error

	var hostname string
	hostname, err = os.Hostname()
	Throw(err)

	return hostname
}

func GetIp() string {
	var err error

	var cmd string
	var cmd_result string

	cmd = "ip -family inet address"
	cmd_result, err = ExecCmdWithTimeout(cmd)
	if err != nil {
		log.Println("cmd:", cmd)
		log.Println("cmd_result:", cmd_result)
	}
	Throw(err)

	var re *regexp.Regexp
	var re_result [][]string

	re = regexp.MustCompile(`inet\s*(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})`)
	re_result = re.FindAllStringSubmatch(cmd_result, -1)

	var ips []string
	var value []string

	for _, value = range re_result {
		if value[1] != "127.0.0.1" {
			ips = append(ips, value[1])
		}
	}

	var ip string
	ip = strings.Join(ips, ",")

	return ip
}

func GetOsType() string {
	var err error

	var filename string
	filename = "/etc/issue"
	_, err = os.Stat("/etc/centos-release")
	if err == nil {
		filename = "/etc/centos-release"
	} else {
		_, err = os.Stat("/etc/redhat-release")
		if err == nil {
			filename = "/etc/redhat-release"
		} else {
		}
	}

	var content []byte
	content, err = ioutil.ReadFile(filename)
	Throw(err)

	var lines []string
	lines = strings.Split(string(content), "\n")

	var os_type string
	if len(lines) > 0 {
		os_type = strings.TrimSpace(lines[0])
	}

	return os_type
}

func GetArchitecture() string {
	var err error

	var cmd string
	var cmd_result string

	cmd = `getconf LONG_BIT |head -c -1; echo -n "-bit "; uname -rm`
	cmd_result, err = ExecCmdWithTimeout(cmd)
	if err != nil {
		log.Println("cmd:", cmd)
		log.Println("cmd_result:", cmd_result)
	}
	Throw(err)

	var architecture string
	architecture = strings.TrimSpace(cmd_result)

	return architecture
}

func GetCpuProcessors() int64 {
	var err error

	var file *os.File
	file, err = os.Open("/proc/cpuinfo")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var cpu_processors int64
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "processor") {
			cpu_processors += 1
		}
	}
	err = scanner.Err()
	Throw(err)

	return cpu_processors
}

// https://www.kernel.org/doc/Documentation/filesystems/proc.txt
func GetMemSize() int64 {
	var err error

	var file *os.File
	file, err = os.Open("/proc/meminfo")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var mem_size int64
	for scanner.Scan() {
		var text string
		text = scanner.Text()

		if strings.HasPrefix(text, "MemTotal:") {
			mem_size, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
			break
		}
	}
	err = scanner.Err()
	Throw(err)

	// GiB
	mem_size = int64(math.Round(float64(mem_size) / (1024 * 1024)))

	return mem_size
}

// https://www.kernel.org/doc/Documentation/filesystems/proc.txt
func GetSwapSize() int64 {
	var err error

	var file *os.File
	file, err = os.Open("/proc/meminfo")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var swap_size int64
	for scanner.Scan() {
		var text string
		text = scanner.Text()

		if strings.HasPrefix(text, "SwapTotal:") {
			swap_size, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
			break
		}
	}
	err = scanner.Err()
	Throw(err)

	// GiB
	swap_size = int64(math.Round(float64(swap_size) / (1024 * 1024)))

	return swap_size
}

// See GetDiskUsage()
func GetDiskSize() int64 {
	var err error

	var file *os.File
	file, err = os.Open("/proc/mounts")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var disk_size uint64
	for scanner.Scan() {
		var text string
		text = scanner.Text()

		// /dev/loop0
		if strings.HasPrefix(text, "/dev") &&
			!strings.Contains(text, "/dev/loop") &&
			!strings.Contains(text, "chroot") &&
			!strings.Contains(text, "docker") {
			var mount_point string
			mount_point = strings.Fields(text)[1]

			var stat syscall.Statfs_t
			syscall.Statfs(mount_point, &stat)

			disk_size += uint64(stat.Bsize) * stat.Blocks
		}
	}
	err = scanner.Err()
	Throw(err)

	// GiB
	//
	// On Aliyun ECS
	// math.Round() returns 39G
	// math.Ceil() returns 40G
	var disk_size2 int64
	disk_size2 = int64(math.Ceil(float64(disk_size) / (1024 * 1024 * 1024)))

	return disk_size2
}

func GetUptime() float64 {
	var err error

	var content []byte
	content, err = ioutil.ReadFile("/proc/uptime")
	Throw(err)

	var uptime float64
	uptime, err = strconv.ParseFloat(strings.Fields(string(content))[0], 64)
	Throw(err)

	// days
	uptime = math.Round(uptime/(3600*24)*100) / 100

	return uptime
}

func GetLoadavg() (float64, float64, float64) {
	var err error

	var content []byte
	content, err = ioutil.ReadFile("/proc/loadavg")
	Throw(err)

	var fields []string
	fields = strings.Fields(string(content))

	// // 1m, 5m, 15m
	// var loadavg string
	// loadavg = fmt.Sprintf("%s,%s,%s", fields[0], fields[1], fields[2])
	// return loadavg

	var loadavg_1m float64
	var loadavg_5m float64
	var loadavg_15m float64

	loadavg_1m, err = strconv.ParseFloat(fields[0], 64)
	Throw(err)
	loadavg_5m, err = strconv.ParseFloat(fields[1], 64)
	Throw(err)
	loadavg_15m, err = strconv.ParseFloat(fields[2], 64)
	Throw(err)

	return loadavg_1m, loadavg_5m, loadavg_15m
}

//  1: user
//  2: nice
//  3: system
//  4: idle
//  5: iowait
//  6: irq
//  7: softirq
//  8: steal
//  9: guest
// 10: guest_nice
//
// https://www.kernel.org/doc/Documentation/filesystems/proc.txt
func GetCpuUsage() (float64, float64) {
	var CalculateCpuUsage func() (int64, int64, int64)
	CalculateCpuUsage = func() (int64, int64, int64) {
		var err error

		var file *os.File
		file, err = os.Open("/proc/stat")
		defer file.Close()
		Throw(err)

		var scanner *bufio.Scanner
		scanner = bufio.NewScanner(file)

		var fields []string
		for scanner.Scan() {
			fields = strings.Fields(scanner.Text())
			break
		}
		err = scanner.Err()
		Throw(err)

		var user int64
		var nice int64
		var system int64
		var idle int64
		var iowait int64
		var irq int64
		var softirq int64

		user, err = strconv.ParseInt(fields[1], 10, 64)
		Throw(err)
		nice, err = strconv.ParseInt(fields[2], 10, 64)
		Throw(err)
		system, err = strconv.ParseInt(fields[3], 10, 64)
		Throw(err)
		idle, err = strconv.ParseInt(fields[4], 10, 64)
		Throw(err)
		iowait, err = strconv.ParseInt(fields[5], 10, 64)
		Throw(err)
		irq, err = strconv.ParseInt(fields[6], 10, 64)
		Throw(err)
		softirq, err = strconv.ParseInt(fields[7], 10, 64)
		Throw(err)

		var used int64
		var total int64

		used = user + nice + system
		total = user + nice + system + idle + iowait + irq + softirq

		return used, iowait, total
	}

	var used int64
	var iowait int64
	var total int64
	var used2 int64
	var iowait2 int64
	var total2 int64

	used, iowait, total = CalculateCpuUsage()
	time.Sleep(1 * time.Second)
	used2, iowait2, total2 = CalculateCpuUsage()

	var cpu_used float64
	var cpu_iowait float64

	cpu_used = float64(used2-used) / float64(total2-total)
	cpu_iowait = float64(iowait2-iowait) / float64(total2-total)

	// var cpu_usage string
	// cpu_usage = fmt.Sprintf("%.2f,%.2f", cpu_used*100, cpu_iowait*100)
	// return cpu_usage

	cpu_used = math.Round(cpu_used*100*100) / 100
	cpu_iowait = math.Round(cpu_iowait*100*100) / 100

	return cpu_used, cpu_iowait
}

// https://www.kernel.org/doc/Documentation/filesystems/proc.txt
func GetMemUsage() (float64, float64) {
	var err error

	var file *os.File
	file, err = os.Open("/proc/meminfo")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var mem_total int64
	var memfree int64
	var buffers int64
	var cached int64
	var swap_total int64
	var swap_free int64

	for scanner.Scan() {
		var text string
		text = scanner.Text()

		if strings.HasPrefix(text, "MemTotal:") {
			mem_total, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
		} else if strings.HasPrefix(text, "MemFree") {
			memfree, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
		} else if strings.HasPrefix(text, "Buffers") {
			buffers, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
		} else if strings.HasPrefix(text, "Cached") {
			cached, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
		} else if strings.HasPrefix(text, "SwapTotal") {
			swap_total, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
		} else if strings.HasPrefix(text, "SwapFree") {
			swap_free, err = strconv.ParseInt(strings.Fields(text)[1], 10, 64)
			Throw(err)
		}
	}
	err = scanner.Err()
	Throw(err)

	// GiB
	// var mem_total2 float64
	// var swap_total2 float64
	// mem_total2 = math.Round(float64(mem_total) / (1024 * 1024))
	// swap_total2 = math.Round(float64(swap_total) / (1024 * 1024))

	var mem_used float64
	var swap_used float64

	mem_used = float64(mem_total-memfree-buffers-cached) / float64(mem_total) * 100
	swap_used = float64(swap_total-swap_free) / (float64(swap_total) + 0.1) * 100

	// var mem_usage string
	// mem_usage = fmt.Sprintf("%.0f,%.2f,%.0f,%.2f", mem_total2, mem_used, swap_total2, swap_used)
	// return mem_usage

	mem_used = math.Round(mem_used*100) / 100
	swap_used = math.Round(swap_used*100) / 100

	return mem_used, swap_used
}

// type Statfs_t struct {
//     Type    int64
//     Bsize   int64
//     Blocks  uint64
//     Bfree   uint64
//     Bavail  uint64
//     Files   uint64
//     Ffree   uint64
//     Fsid    Fsid
//     Namelen int64
//     Frsize  int64
//     Flags   int64
//     Spare   [4]int64
// }
//
// https://man7.org/linux/man-pages/man3/statvfs.3.html
// https://pkg.go.dev/syscall#Statfs_t
func GetDiskUsage() (string, float64, float64) {
	var err error

	var file *os.File
	file, err = os.Open("/proc/mounts")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var disks []string
	var max_disk_used float64
	var max_inode_used float64

	for scanner.Scan() {
		var text string
		text = scanner.Text()

		// /dev/loop0
		if strings.HasPrefix(text, "/dev") &&
			!strings.Contains(text, "/dev/loop") &&
			!strings.Contains(text, "chroot") &&
			!strings.Contains(text, "docker") {
			var mount_point string
			mount_point = strings.Fields(text)[1]

			var stat syscall.Statfs_t
			syscall.Statfs(mount_point, &stat)

			var disk_total float64
			var disk_used float64

			// GiB
			disk_total = math.Round(float64(uint64(stat.Frsize)*stat.Blocks) / (1024 * 1024 * 1024))
			disk_used = float64(uint64(stat.Frsize)*(stat.Blocks-stat.Bfree)) / float64(uint64(stat.Frsize)*stat.Blocks) * 100

			if disk_used > max_disk_used {
				max_disk_used = disk_used
			}

			var inode_used float64
			if stat.Files != 0 {
				inode_used = float64(stat.Files-stat.Ffree) / float64(stat.Files) * 100

				if inode_used > max_inode_used {
					max_inode_used = inode_used
				}
			}

			var disk string
			disk = fmt.Sprintf("%s_%.0f_%.2f_%.2f", mount_point, disk_total, disk_used, inode_used)

			disks = append(disks, disk)
		}
	}
	err = scanner.Err()
	Throw(err)

	var disk_usage string
	disk_usage = strings.Join(disks, ",")

	max_disk_used = math.Round(max_disk_used*100) / 100
	max_inode_used = math.Round(max_inode_used*100) / 100

	return disk_usage, max_disk_used, max_inode_used
}

//  0: major number
//  1: minor mumber
//  2: device name
//  3: reads completed successfully
//  4: reads merged
//  5: sectors read
//  6: time spent reading (ms)
//  7: writes completed
//  8: writes merged
//  9: sectors written
// 10: time spent writing (ms)
// 11: I/Os currently in progress
// 12: time spent doing I/Os (ms)
// 13: weighted time spent doing I/Os (ms)
//
// https://www.kernel.org/doc/Documentation/iostats.txt
// https://www.kernel.org/doc/Documentation/ABI/testing/procfs-diskstats
func GetDiskIoRate() (float64, float64, int64) {
	var CalculateDiskIoRate func(re *regexp.Regexp) (int64, int64, int64)
	CalculateDiskIoRate = func(re *regexp.Regexp) (int64, int64, int64) {
		var err error

		var file *os.File
		file, err = os.Open("/proc/diskstats")
		defer file.Close()
		Throw(err)

		var scanner *bufio.Scanner
		scanner = bufio.NewScanner(file)

		var rsectors int64
		var wsectors int64
		var ios int64

		for scanner.Scan() {
			var text string
			text = scanner.Text()

			if re.MatchString(text) {
				var fields []string
				var rsectors2 int64
				var wsectors2 int64
				var ios2 int64

				fields = strings.Fields(text)
				rsectors2, err = strconv.ParseInt(fields[5], 10, 64)
				Throw(err)
				wsectors2, err = strconv.ParseInt(fields[9], 10, 64)
				Throw(err)
				ios2, err = strconv.ParseInt(fields[11], 10, 64)
				Throw(err)

				rsectors += rsectors2
				wsectors += wsectors2
				ios += ios2
			}
		}
		err = scanner.Err()
		Throw(err)

		return rsectors, wsectors, ios
	}

	var err error

	var file *os.File
	file, err = os.Open("/proc/diskstats")
	defer file.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var read_rate float64
	var write_rate float64
	var ios int64

	var re *regexp.Regexp
	var is_matched bool

	for scanner.Scan() {
		var text string
		text = scanner.Text()

		var patterns []string
		patterns = []string{
			" sd[a-z] ",
			" sd[a-z][0-9] ",
			" vd[a-z] ",
			" vd[a-z][0-9] ",
			" xvd[a-z] ",
			" xvd[a-z][0-9] ",
			" hd[a-z] ",
			" hd[a-z][0-9] ",
		}

		var pattern string
		for _, pattern = range patterns {
			re = regexp.MustCompile(pattern)

			if re.MatchString(text) {
				is_matched = true
				break
			}
		}

		if is_matched {
			break
		}
	}
	err = scanner.Err()
	Throw(err)

	var rsectors int64
	var wsectors int64
	var rsectors2 int64
	var wsectors2 int64

	rsectors, wsectors, _ = CalculateDiskIoRate(re)
	time.Sleep(1 * time.Second)
	rsectors2, wsectors2, ios = CalculateDiskIoRate(re)

	// KiB/s
	read_rate = float64(rsectors2-rsectors) * 512 / 1024
	write_rate = float64(wsectors2-wsectors) * 512 / 1024

	// var disk_io_rate string
	// disk_io_rate = fmt.Sprintf("%.2f,%.2f,%d", read_rate, write_rate, ios)
	// return disk_io_rate

	var disk_read_rate float64
	var disk_write_rate float64
	var disk_ios int64

	disk_read_rate = math.Round(read_rate*100) / 100
	disk_write_rate = math.Round(write_rate*100) / 100
	disk_ios = ios

	return disk_read_rate, disk_write_rate, disk_ios
}

//  0: Interface
//  1: Receive bytes
//  2: Receive packets
//  3: Receive errs
//  4: Receive drop
//  5: Receive fifo
//  6: Receive frame
//  7: Receive compressed
//  8: Receive multicast
//  9: Transmit bytes
// 10: Transmit packets
// 11: Transmit errs
// 12: Transmit drop
// 13: Transmit fifo
// 14: Transmit frame
// 15: Transmit compressed
// 16: Transmit multicast
//
// https://www.kernel.org/doc/Documentation/filesystems/proc.txt
func GetNicIoRate() (float64, int64, float64, int64) {
	var CalculateNicIoRate func() (int64, int64, int64, int64)
	CalculateNicIoRate = func() (int64, int64, int64, int64) {
		var err error

		var file *os.File
		file, err = os.Open("/proc/net/dev")
		defer file.Close()
		Throw(err)

		var scanner *bufio.Scanner
		scanner = bufio.NewScanner(file)

		var receive_bytes int64
		var transmit_bytes int64
		var receive_packets int64
		var transmit_packets int64

		for scanner.Scan() {
			var text string
			text = scanner.Text()

			if !strings.Contains(text, "Inter") &&
				!strings.Contains(text, "face") &&
				!strings.Contains(text, "lo:") {
				var fields []string
				var receive_bytes2 int64
				var receive_packets2 int64
				var transmit_bytes2 int64
				var transmit_packets2 int64

				fields = strings.Fields(text)
				receive_bytes2, err = strconv.ParseInt(fields[1], 10, 64)
				Throw(err)
				receive_packets2, err = strconv.ParseInt(fields[2], 10, 64)
				Throw(err)
				transmit_bytes2, err = strconv.ParseInt(fields[9], 10, 64)
				Throw(err)
				transmit_packets2, err = strconv.ParseInt(fields[10], 10, 64)
				Throw(err)

				receive_bytes += receive_bytes2
				receive_packets += receive_packets2
				transmit_bytes += transmit_bytes2
				transmit_packets += transmit_packets2
			}
		}
		err = scanner.Err()
		Throw(err)

		return receive_bytes, transmit_bytes, receive_packets, transmit_packets
	}

	var receive_bytes int64
	var transmit_bytes int64
	var receive_packets int64
	var transmit_packets int64
	var receive_bytes2 int64
	var transmit_bytes2 int64
	var receive_packets2 int64
	var transmit_packets2 int64

	receive_bytes, transmit_bytes, receive_packets, transmit_packets = CalculateNicIoRate()
	time.Sleep(1 * time.Second)
	receive_bytes2, transmit_bytes2, receive_packets2, transmit_packets2 = CalculateNicIoRate()

	var receive_rate float64
	var receive_packets3 int64
	var transmit_rate float64
	var transmit_packets3 int64

	// KiB/s
	receive_rate = float64(receive_bytes2-receive_bytes) / 1024
	receive_packets3 = receive_packets2 - receive_packets
	transmit_rate = float64(transmit_bytes2-transmit_bytes) / 1024
	transmit_packets3 = transmit_packets2 - transmit_packets

	// var nic_io_rate string
	// nic_io_rate = fmt.Sprintf("%.2f,%d,%.2f,%d", receive_rate, receive_packets3, transmit_rate, transmit_packets3)
	// return nic_io_rate

	var nic_receive_rate float64
	var nic_receive_packets int64
	var nic_transmit_rate float64
	var nic_transmit_packets int64

	nic_receive_rate = math.Round(receive_rate*100) / 100
	nic_receive_packets = receive_packets3
	nic_transmit_rate = math.Round(transmit_rate*100) / 100
	nic_transmit_packets = transmit_packets3

	return nic_receive_rate, nic_receive_packets, nic_transmit_rate, nic_transmit_packets
}

func GetTcpSockets() (int64, int64) {
	var err error

	var inuse int64
	var tw int64

	{
		var file *os.File
		file, err = os.Open("/proc/net/sockstat")
		defer file.Close()
		Throw(err)

		var scanner *bufio.Scanner
		scanner = bufio.NewScanner(file)

		for scanner.Scan() {
			var text string
			text = scanner.Text()

			if strings.HasPrefix(text, "TCP:") {
				var fields []string
				var inuse2 int64
				var tw2 int64

				fields = strings.Fields(text)
				inuse2, err = strconv.ParseInt(fields[2], 10, 64)
				Throw(err)
				tw2, err = strconv.ParseInt(fields[6], 10, 64)
				Throw(err)

				inuse += inuse2
				tw += tw2

				break
			}
		}
		err = scanner.Err()
		Throw(err)
	}

	{
		_, err = os.Stat("/proc/net/sockstat6")
		if err == nil {
			var file *os.File
			file, err = os.Open("/proc/net/sockstat6")
			defer file.Close()
			Throw(err)

			var scanner *bufio.Scanner
			scanner = bufio.NewScanner(file)

			for scanner.Scan() {
				var text string
				text = scanner.Text()

				if strings.HasPrefix(text, "TCP6:") {
					var fields []string
					var inuse2 int64

					fields = strings.Fields(text)
					inuse2, err = strconv.ParseInt(fields[2], 10, 64)
					Throw(err)

					inuse += inuse2

					break
				}
			}
			err = scanner.Err()
			Throw(err)
		} else if os.IsNotExist(err) {
		} else {
		}
	}

	// var tcp_sockets string
	// tcp_sockets = fmt.Sprintf("%d,%d", inuse, tw)
	// return tcp_sockets

	var tcp_sockets_inuse int64
	var tcp_sockets_tw int64

	tcp_sockets_inuse = inuse
	tcp_sockets_tw = tw

	return tcp_sockets_inuse, tcp_sockets_tw
}

func GetUsers() int64 {
	var err error

	var cmd string
	var cmd_result string

	cmd = "users"
	cmd_result, err = ExecCmdWithTimeout(cmd)
	if err != nil {
		log.Println("cmd:", cmd)
		log.Println("cmd_result:", cmd_result)
	}
	Throw(err)

	var users int64
	users = int64(len(strings.Fields(cmd_result)))

	return users
}

func GetCurrentTime() string {
	var current_time string
	current_time = time.Now().Format("2006-01-02 15:04:05")
	return current_time
}

// code
// hostname
// ip
// os_type
// architecture
// cpu_processors
// mem_size
// swap_size
// disk_size
// uptime
// heartbeat_time
// project
// version
func GetHost() []byte {
	defer Catch()

	var err error

	var host map[string]interface{}
	host = map[string]interface{}{
		"code":           GetCode(),
		"hostname":       GetHostname(),
		"ip":             GetIp(),
		"os_type":        GetOsType(),
		"architecture":   GetArchitecture(),
		"cpu_processors": GetCpuProcessors(),
		"mem_size":       GetMemSize(),
		"swap_size":      GetSwapSize(),
		"disk_size":      GetDiskSize(),
		"uptime":         GetUptime(),
		"heartbeat_time": GetCurrentTime(),
		"project":        SETTINGS.PROJECT,
		"version":        SETTINGS.VERSION,
	}

	if SETTINGS.DEBUG {
		var tmp []byte
		tmp, err = json.MarshalIndent(host, "", "    ")
		Skip(err)
		log.Println("host: \n" + string(tmp))
	}

	var host2 []byte
	host2, err = json.Marshal(host)
	Throw(err)

	return host2
}

// code
// hostname
// loadavg: loadavg_1m, loadavg_5m, loadavg_15m
// cpu_usage: cpu_used, cpu_iowait
// mem_usage: mem_used, swap_used
// disk_usage: disk_used, inode_used
// disk_io_rate: disk_read_rate, disk_write_rate, disk_ios
// nic_io_rate: nic_receive_rate, nic_receive_packets, nic_transmit_rate, nic_transmit_packets
// tcp_sockets: tcp_sockets_inuse, tcp_sockets_tw
// users
// heartbeat_time
// project
func GetHostMetric() []byte {
	defer Catch()

	var err error

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
	var disk_ios int64
	var nic_receive_rate float64
	var nic_receive_packets int64
	var nic_transmit_rate float64
	var nic_transmit_packets int64
	var tcp_sockets_inuse int64
	var tcp_sockets_tw int64
	var users int64
	var heartbeat_time string
	var project string

	code = GetCode()
	hostname = GetHostname()
	ip = GetIp()
	loadavg_1m, loadavg_5m, loadavg_15m = GetLoadavg()
	cpu_used, cpu_iowait = GetCpuUsage()
	mem_used, swap_used = GetMemUsage()
	disk_usage, disk_used, inode_used = GetDiskUsage()
	disk_read_rate, disk_write_rate, disk_ios = GetDiskIoRate()
	nic_receive_rate, nic_receive_packets, nic_transmit_rate, nic_transmit_packets = GetNicIoRate()
	tcp_sockets_inuse, tcp_sockets_tw = GetTcpSockets()
	users = GetUsers()
	heartbeat_time = GetCurrentTime()
	project = SETTINGS.PROJECT

	var host_metric map[string]interface{}
	host_metric = map[string]interface{}{
		"code":                 code,
		"hostname":             hostname,
		"ip":                   ip,
		"loadavg_1m":           loadavg_1m,
		"loadavg_5m":           loadavg_5m,
		"loadavg_15m":          loadavg_15m,
		"cpu_used":             cpu_used,
		"cpu_iowait":           cpu_iowait,
		"mem_used":             mem_used,
		"swap_used":            swap_used,
		"disk_usage":           disk_usage,
		"disk_used":            disk_used,
		"inode_used":           inode_used,
		"disk_read_rate":       disk_read_rate,
		"disk_write_rate":      disk_write_rate,
		"disk_ios":             disk_ios,
		"nic_receive_rate":     nic_receive_rate,
		"nic_receive_packets":  nic_receive_packets,
		"nic_transmit_rate":    nic_transmit_rate,
		"nic_transmit_packets": nic_transmit_packets,
		"tcp_sockets_inuse":    tcp_sockets_inuse,
		"tcp_sockets_tw":       tcp_sockets_tw,
		"users":                users,
		"heartbeat_time":       heartbeat_time,
		"project":              project,
	}

	if SETTINGS.DEBUG {
		var tmp []byte
		tmp, err = json.MarshalIndent(host_metric, "", "    ")
		Skip(err)
		log.Println("host_metric: \n" + string(tmp))
	}

	var host_metric2 []byte
	host_metric2, err = json.Marshal(host_metric)
	Throw(err)

	return host_metric2
}

func ReportHost(wg *sync.WaitGroup) {
	defer wg.Done()

	var api string
	api = fmt.Sprintf("%s/report_host", SETTINGS.API)

	for {
		var host []byte
		host = GetHost()

		log.Println("api:", api)
		log.Println("host:", string(host))

		if len(host) > 0 {
			HttpPost(api, host)
		} else {
			log.Println("get host failed")
		}

		if SETTINGS.DEBUG {
			time.Sleep(5 * time.Second)
		} else {
			time.Sleep(5 * time.Minute)
		}
	}
}

func ReportHostMetric(wg *sync.WaitGroup) {
	defer wg.Done()

	var api string
	api = fmt.Sprintf("%s/report_host_metric", SETTINGS.API)

	for {
		var host_metric []byte
		host_metric = GetHostMetric()

		log.Println("api:", api)
		log.Println("host_metric:", string(host_metric))

		if len(host_metric) > 0 {
			HttpPost(api, host_metric)
		} else {
			log.Println("get host metric failed")
		}

		if SETTINGS.DEBUG {
			time.Sleep(1 * time.Second)
		} else {
			time.Sleep(1 * time.Minute)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	var host string
	var port int
	var project string
	var debug bool

	flag.StringVar(&host, "host", "127.0.0.1", "Host")
	flag.IntVar(&port, "port", 1234, "Port")
	flag.StringVar(&project, "project", "DEFAULT", "Project")
	flag.BoolVar(&debug, "debug", false, "Debug")

	flag.Parse()

	log.Println("host:", host)
	log.Println("port:", port)
	log.Println("project:", project)
	log.Println("debug:", debug)

	SETTINGS.API = fmt.Sprintf("http://%s:%d/api", host, port)
	SETTINGS.PROJECT = project
	SETTINGS.DEBUG = debug

	log.Printf("SETTINGS: %+v\n", SETTINGS)

	var wg sync.WaitGroup
	wg.Add(1)
	go ReportHost(&wg)
	wg.Add(1)
	go ReportHostMetric(&wg)
	wg.Wait()
}
