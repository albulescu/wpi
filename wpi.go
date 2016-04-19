package main

import (
	"fmt"
	"bufio"
	"net"
	"log"
	"strings"
	"bytes"
	"strconv"
)


type hub struct {
	connections map[*connection]bool
	register chan *connection
	unregister chan *connection
}

type connection struct {
	conn net.Conn
	send chan string
	token string
}

func (c *connection) String() string {
	return c.conn.RemoteAddr().String();
}

func (c *connection) auth( token string) {
	fmt.Println("Auth with:", token)
	if token == "abc" {
		c.token = token
		fmt.Println("Auth OK")
		c.conn.Write([]byte("1\n"))
	} else {
		fmt.Println("Auth FAIL")
		c.conn.Write([]byte("0\n"))
		c.conn.Close()
	}
}

func (c *connection) error(message string) {
	fmt.Println("Error:",message)
	c.conn.Write([]byte(message))
	c.conn.Close()
}

func (c *connection) readPump() {

	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	reader := bufio.NewReader(c.conn)

	var importing string;
	var size int;
	var contents bytes.Buffer;

	for {

		message, err := reader.ReadBytes('\n')

		/**
		 If we are in importing state write to
		 buffer until END command received
		 */
		if importing != "" {

			if strings.Index(string(message),"IMPORT") == 0 && contents.Len() == 0 {
				c.error("ERR:IMPORTING")
				return
			}

			if string(message) == "END\n" {

				if contents.Len() == 0 {
					c.error("ERR:ZEROFSIZE")
					return
				}

				fmt.Println("Import comlete:", importing, "with size", contents.Len())
				c.conn.Write([]byte("1\n"))
				importing = ""
				contents.Reset();
				continue
			}

			contents.Write(message)

			continue;
		}

		if err != nil {
			break
		}

		request := strings.SplitN(string(message), " ", 2)

		if len(request) == 0 {
			break
		}

		var command string = request[0];
		var param string

		if len(request) == 2 {
			param = strings.TrimRight(request[1],"\n")
		}

		if command != "AUTH" && c.token == "" {
			c.error("ERR:NOAUTH")
			return
		}

		if command == "AUTH" {
			c.auth(param);
		} else if command == "IMPORT" {

			impinfo := strings.Split(param,"|");

			importing=impinfo[0];
			size,_=strconv.Atoi(impinfo[1]);

			c.conn.Write([]byte("1\n"))
			fmt.Println("Start import", importing, "with size", size)
		} else {
			fmt.Println("Unknown command", request[0]);
		}
	}
}

func (c *connection) write(payload []byte) error {
	return nil
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
				log.Println("Fail to send data")
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
			//log.Println("Register user:", c.String())
		case c := <-h.unregister:
			//log.Println("Unregister user:", c.String())
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

		c := &connection{conn:con, send: make(chan string)}

		h.register <- c

		go c.writePump()
		c.readPump()
	}

}
