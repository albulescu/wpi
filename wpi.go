package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
)

type hub struct {
	connections map[*connection]bool
	register    chan *connection
	unregister  chan *connection
}

type connection struct {
	conn   net.Conn
	send   chan string
	token  string
	params map[string]string
}

func (c *connection) String() string {
	return c.conn.RemoteAddr().String()
}

func (c *connection) auth(token string) {
	fmt.Println("Auth with:", token)
	if token == "abc" {
		c.token = token
		fmt.Println("Auth OK")
		c.send <- "OK"
		h.register <- c
	} else {
		fmt.Println("Auth FAIL")
		c.send <- "FAIL"
		c.conn.Close()
	}
}

func (c *connection) set(data string) {

	if c.params == nil {
		c.params = make(map[string]string)
	}

	part := strings.SplitN(data, " ", 2)

	if len(part) != 2 {
		c.error("INVALID_SET")
		return
	}

	log.Println("SET ", data)

	c.params[part[0]] = part[1]
	c.send <- "OK"
}

func (c *connection) error(message string) {
	fmt.Println("Error:", message)
	c.send <- message
	c.conn.Close()
}

func (c *connection) readPump() {

	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	reader := bufio.NewReader(c.conn)

	var importing string
	var size int
	var contents bytes.Buffer

	for {

		message, err := reader.ReadBytes('\n')

		if err != nil {
			fmt.Println(err.Error())
			break
		}

		/**
		If we are in importing state write to
		buffer until END command received
		*/
		if importing != "" {

			if strings.Index(string(message), "IMPORT") == 0 && contents.Len() == 0 {
				c.error("ERR:IMPORTING")
				return
			}

			if string(message) == "END\n" {

				if contents.Len() == 0 {
					c.error("ZEROFSIZE")
					return
				}

				contents.Truncate(size)

				percent := contents.Len()/size*100

				fmt.Print(" - ",percent)
				fmt.Print("%")
				if contents.Len() != size {
					fmt.Print(" ( ! )")
				}
				fmt.Print("\n")

				c.send <- "OK"

				importing = ""
				contents.Reset()
				continue
			}

			contents.Write(message)

			continue
		}

		var command string = strings.TrimRight(string(message), "\n")
		var param string

		if strings.Index(string(message), " ") != -1 {
			request := strings.SplitN(string(message), " ", 2)
			command = request[0]
			if len(request) == 2 {
				param = strings.TrimRight(request[1], "\n")
			}
		}

		if command != "AUTH" && c.token == "" {
			c.error("NOAUTH")
			return
		}

		if command == "AUTH" {
			c.auth(param)
		} else if command == "SET" {
			c.set(param)
		} else if command == "FINISH" {
			log.Println("Finish Import")
			c.conn.Write([]byte("OK"))
			return
		} else if command == "IMPORT" {
			impinfo := strings.Split(param, "|")
			importing = impinfo[0]
			size, _ = strconv.Atoi(impinfo[1])
			fmt.Print("> ",importing)
			c.send <- "OK"
		} else {
			fmt.Println("Unknown command", command)
		}
	}
}

func (c *connection) writePump() {
	defer func() {
		c.conn.Close()
	}()
	for {
		select {
		case message, ok := <-c.send:

			if !ok {
				return
			}

			if _, err := c.conn.Write([]byte(message + "\n")); err != nil {
				log.Println("Fail to send data:",message)
				return
			}
		}
	}
}

var h = hub{
	register:    make(chan *connection),
	unregister:  make(chan *connection),
	connections: make(map[*connection]bool),
}

func (h *hub) run() {
	for {
		select {
		case c := <-h.register:
			h.connections[c] = true
			log.Println("Register user:", c.String())
		case c := <-h.unregister:
			log.Println("Unregister user:", c.String())
			if _, ok := h.connections[c]; ok {
				delete(h.connections, c)
				close(c.send)
			}
		}
	}
}

func main() {

	ln, err := net.Listen("tcp", ":9999")

	if err != nil {
		panic(err)
	}

	go h.run()

	for {
		con, err := ln.Accept()

		if err != nil {
			panic(err)
		}

		c := &connection{conn: con, send: make(chan string)}

		go c.writePump()
		c.readPump()
	}

}
