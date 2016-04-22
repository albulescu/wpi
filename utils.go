package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return true, err
}

func completeJson(url string) []byte {
	data := make(map[string]string, 0)
	data["url"] = url
	json, err := json.Marshal(data)
	if err != nil {
		panic(err)
	}
	return json
}

func createEmptyFile(name string) error {
	fo, err := os.Create(name)
	if err != nil {
		return err
	}
	defer func() {
		fo.Close()
	}()
	return nil
}

func prepareFilePath(c *connection, file string) string {

	prop, err := c.get("url")

	if err != nil {
		panic(err)
	}

	u, err := url.Parse(prop)

	if err != nil {
		panic(err)
	}

	host := strings.Split(u.Host, ":")[0]

	var writePathBuffer bytes.Buffer

	tempPath, err := filepath.Abs(config.Temp)

	if err != nil {
		panic(err)
	}

	writePathBuffer.WriteString(tempPath)
	writePathBuffer.WriteString("/")
	writePathBuffer.WriteString(host)

	siteExist, err := exists(writePathBuffer.String())

	if err != nil {
		panic(err)
	}

	if !siteExist {
		if verbose {
			fmt.Println("Create directory: ", writePathBuffer.String())
		}
		if err := os.MkdirAll(writePathBuffer.String(), 0777); err != nil {
			panic(err)
		}
	}

	var filePath bytes.Buffer

	filePath.WriteString(writePathBuffer.String())
	filePath.WriteString("/")
	filePath.WriteString(file)

	fExists, err := exists(path.Dir(filePath.String()))

	if err != nil {
		panic(err)
	}

	if !fExists {
		if verbose {
			fmt.Println("Create directory: ", path.Dir(filePath.String()))
		}
		if err := os.MkdirAll(path.Dir(filePath.String()), 0777); err != nil {
			panic(err)
		}
	}

	return filePath.String()
}

func writeFile(c *connection, file string, buffer bytes.Buffer) {

	filePath := prepareFilePath(c, file)

	if err := ioutil.WriteFile(filePath, buffer.Bytes(), 0777); err != nil {
		fmt.Println("Fail to write file")
		panic(err)
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

func concat(strs ...string) string {
	var buffer bytes.Buffer
	for _, str := range strs {
		buffer.WriteString(str)
	}
	return buffer.String()
}
