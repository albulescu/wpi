package main

import (
	"flag"
	"fmt"
	"gopkg.in/ini.v1"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
)

var VERSION string = "0.0.0"
var DEBUG bool = false
var verbose bool = true

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
	Secret       string `ini:"secret"`
}

var config *Configuration = &Configuration{}

func main() {

	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println(VERSION)
		os.Exit(0)
	}

	if VERSION == "0.0.0" {
		DEBUG = true
	}

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

	if abspath, err := filepath.Abs(config.Temp); err != nil {
		panic(err)
		os.Exit(1)
	}

	stat, err := os.Stat(config.Temp)

	if os.IsNotExist(err) {
		panic(err)
		os.Exit(1)
	}

	if !stat.IsDir() {
		panic("Temp dir should be a directory. Check config file. from /etc/wpi.conf")
		os.Exit(1)
	}

	fmt.Println("Starting on", config.BindAddress)

	ln, err := net.Listen("tcp", config.BindAddress)

	if err != nil {
		panic(err)
	}

	// some channels
	counters = make(map[*connection]int)
	writeComplete = make(chan *connection)
	writeFiles = make(chan WriteJob, 100)

	// define number of workers
	workersNum := MAX_INDEX_WORKERS
	if config.Workers != 0 {
		workersNum = config.Workers
	}

	fmt.Println("Start with", workersNum, "workers")

	// start workers for writing files
	dispatcher := NewDispatcher(writeFiles, workersNum)
	go dispatcher.Run()

	// start hub that keeps clients
	go h.run()

	// watch for expired imports
	go Clean()

	// watch for completed imports
	go WatchCompleted()

	go (func() {
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
	})()

	ch := make(chan os.Signal)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	fmt.Println(<-ch)

	fmt.Println("Close active connections")
	for c, _ := range h.connections {
		c.conn.Write([]byte("9\n"))
		c.conn.Close()
	}
}
