package main

import (
	"flag"
	"fmt"
	"gopkg.in/ini.v1"
	"net"
	"path/filepath"
	"strconv"
)

var verbose bool = false

var MAX_INDEX_WORKERS int = 100

var writeFiles chan WriteJob

var writeComplete chan *connection

var counters map[*connection]int

type Configuration struct {
	BindAddress  string `ini:"bind"`
	Temp         string `ini:"temp"`
	Workers      int    `ini:"workers"`
	TempLifetime string `ini:"lifetime"`
	Mounts       string `ini:"mounts"`
}

var config *Configuration = &Configuration{}

func main() {

	var configFileFlag = flag.String("config", "", "Config file")

	flag.Parse()

	configFile, err := filepath.Abs(*configFileFlag)

	if err != nil {
		panic("Fail to get absolute file path for ini")
	}

	ini, err := ini.Load(configFile)

	if err != nil {
		panic("Fail to load ini file")
	}

	ini.MapTo(config)

	fmt.Println("Starting on", config.BindAddress)

	ln, err := net.Listen("tcp", config.BindAddress)

	if err != nil {
		panic(err)
	}

	counters = make(map[*connection]int)

	writeComplete = make(chan *connection)
	writeFiles = make(chan WriteJob, 100)

	workersNum := MAX_INDEX_WORKERS

	if config.Workers != 0 {
		workersNum = config.Workers
	}

	fmt.Println("Start with", workersNum, "workers")

	dispatcher := NewDispatcher(writeFiles, workersNum)
	go dispatcher.Run()

	go h.run()

	go (func() {
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
	})()

	for {
		con, err := ln.Accept()

		if err != nil {
			panic(err)
		}

		c := &connection{conn: con, send: make(chan string)}

		counters[c] = 0

		go c.readPump()
		c.writePump()
	}

}
