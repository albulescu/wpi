package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
)

func writeFile(c *connection, file string, buffer bytes.Buffer) {

	prop, err := c.get("url")

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	u, err := url.Parse(prop)

	if err != nil {
		fmt.Println(err.Error())
		return
	}

	host := strings.Split(u.Host, ":")[0]

	if err := os.MkdirAll(host, 0777); err != nil {
		fmt.Println("Fail to create wp dir")
		return
	}

	var filePath bytes.Buffer

	cwd, err := os.Getwd()

	if err != nil {
		fmt.Println("Fail to get cwd")
		return
	}

	filePath.WriteString(cwd)
	filePath.WriteString("/")
	filePath.WriteString(host)
	filePath.WriteString("/")
	filePath.WriteString(file)

	if err := os.MkdirAll(path.Dir(filePath.String()), 0777); err != nil {
		fmt.Println("Fail to create file dir")
		return
	}

	if err = ioutil.WriteFile(filePath.String(), buffer.Bytes(), 0777); err != nil {
		fmt.Println("Fail to write file")
	}

	writeComplete <- c
}

func postProcess(c *connection) {

}

func WatchCompleted() {
	for {
		select {
		case c := <-writeComplete:
			{
				counters[c]++

				totalStr, err := c.get("files")

				if err != nil {
					panic(err)
				}

				total, err := strconv.Atoi(totalStr)

				if err != nil {
					panic(err)
				}

				if counters[c] == total {
					postProcess(c)
					fmt.Println("Write complete! Process data and inform client!")
					c.send <- "FINISH"
					delete(counters, c)
				}
			}
		}
	}
}
