package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func Throw(err error) {
	if err != nil {
		panic(err)
	}
}

func CopyIndexHtml() {
	var err error

	var content []byte
	content, err = ioutil.ReadFile("./static/highcharts.min.js")
	Throw(err)

	var highcharts_new string
	highcharts_new = fmt.Sprintf("<script>%s</script>", string(content))

	var content2 []byte
	content2, err = ioutil.ReadFile("./static/highcharts_boost.min.js")
	Throw(err)

	var highcharts_boost_new string
	highcharts_boost_new = fmt.Sprintf("<script>%s</script>", string(content2))

	var highcharts_old string
	highcharts_old = `<script type="text/javascript" src="./static/highcharts.min.js"></script>`

	var highcharts_boost_old string
	highcharts_boost_old = `<script type="text/javascript" src="./static/highcharts_boost.min.js"></script>`

	var file *os.File
	file, err = os.Open("./template/index.html")
	defer file.Close()
	Throw(err)

	var file2 *os.File
	file2, err = os.Create("./build/index.html")
	defer file2.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var writer *bufio.Writer
	writer = bufio.NewWriter(file2)

	for scanner.Scan() {
		var text string
		text = scanner.Text()

		if strings.HasSuffix(text, highcharts_old) {
			_, err = writer.WriteString(highcharts_new + "\n")
		} else if strings.HasSuffix(text, highcharts_boost_old) {
			_, err = writer.WriteString(highcharts_boost_new + "\n")
		} else {
			_, err = writer.WriteString(text + "\n")
		}
		Throw(err)
	}
	err = scanner.Err()
	Throw(err)

	err = writer.Flush()
	Throw(err)
}

func CopyLnxmonsrvGo() {
	var err error

	var content []byte
	content, err = ioutil.ReadFile("./build/index.html")
	Throw(err)

	var html_new string
	html_new = fmt.Sprintf("HTML = `%s`", string(content))

	var html_old string
	html_old = `HTML = ""`

	var file *os.File
	file, err = os.Open("./lnxmonsrv.go")
	defer file.Close()
	Throw(err)

	var file2 *os.File
	file2, err = os.Create("./build/lnxmonsrv.go")
	defer file2.Close()
	Throw(err)

	var scanner *bufio.Scanner
	scanner = bufio.NewScanner(file)

	var writer *bufio.Writer
	writer = bufio.NewWriter(file2)

	for scanner.Scan() {
		var text string
		text = scanner.Text()

		if strings.HasSuffix(text, html_old) {
			_, err = writer.WriteString(html_new + "\n")
		} else {
			_, err = writer.WriteString(text + "\n")
		}
		Throw(err)
	}
	err = scanner.Err()
	Throw(err)

	err = writer.Flush()
	Throw(err)
}

func CopyLnxmoncliGo() {
	var err error

	var content []byte
	content, err = ioutil.ReadFile("./lnxmoncli.go")
	Throw(err)

	err = ioutil.WriteFile("./build/lnxmoncli.go", content, 0644)
	Throw(err)
}

func DeleteIndexHtml() {
	var err error
	err = os.Remove("./build/index.html")
	Throw(err)
}

func main() {
	CopyIndexHtml()
	CopyLnxmonsrvGo()
	CopyLnxmoncliGo()
	DeleteIndexHtml()
}
