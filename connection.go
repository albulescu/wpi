package main

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"errors"
)

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

func (c *connection) get(name string) (string,error) {
	if c.params == nil {
		c.params = make(map[string]string)
	}
	if val, ok := c.params[name]; ok {
		return val, nil
	}

	return "", errors.New("Property not found")
}

func (c *connection) error(message string) {
	fmt.Println("Error:", message)
	c.send <- message
	c.conn.Close()
}

func (c *connection) readPump() {

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

				writeFiles <- WriteJob{c:c, file:importing, buffer:contents}

				if verbose {

					if contents.Len() != size {
						fmt.Print("Failed:")
					} else {
						fmt.Print("Success:")
					}

					fmt.Print(importing)

					fmt.Print("\n")
				}

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
			return
		} else if command == "IMPORT" {
			impinfo := strings.Split(param, "|")
			importing = impinfo[0]
			size, _ = strconv.Atoi(impinfo[1])

			if verbose {
				fmt.Print("Import:", importing)
			}

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

