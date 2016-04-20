package main

import (
	"net"
	"strconv"
	"fmt"
)

var verbose bool = false;

var MAX_INDEX_WORKERS int = 200

var writeFiles chan WriteJob

var writeComplete chan *connection

var counters map[*connection]int;

func main() {

	ln, err := net.Listen("tcp", ":9999")

	if err != nil {
		panic(err)
	}

	counters = make(map[*connection]int);

	writeComplete = make(chan *connection)
	writeFiles = make(chan WriteJob, 100)
	dispatcher := NewDispatcher(writeFiles, MAX_INDEX_WORKERS)
	go dispatcher.Run()

	go h.run()

	go (func(){
		for {
			select {
			case c := <-writeComplete:
				{
					counters[c]++;

					totalStr, err := c.get("files")

					if err != nil {
						panic(err)
					}

					total, err := strconv.Atoi(totalStr);

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
	})();

	for {
		con, err := ln.Accept()

		if err != nil {
			panic(err)
		}

		c := &connection{conn: con, send: make(chan string)}

		counters[c]=0;

		go c.readPump()
		c.writePump()
	}

}
