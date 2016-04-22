package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
)

var OK string = "0"
var CHUNK_SIZE = 1024

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
		c.send <- "0"
		h.register <- c
	} else {
		fmt.Println("Auth FAIL")
		c.send <- "1"
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

	c.send <- OK
}

func (c *connection) get(name string) (string, error) {
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

	reader := bufio.NewReaderSize(c.conn, 1024)

	var importing string
	var size int
	var crc string

	var contents bytes.Buffer

	for {
		/**
		If we are in importing state write to
		buffer until END command received
		*/
		if importing != "" {

			var chunk []byte = make([]byte, 1024)

			n, err := reader.Read(chunk)

			if err == io.EOF {
				return
			}

			if err != nil {
				panic(err)
			}

			if n == 0 {
				continue
			}

			cntWriteNum, cntWriteErr := contents.Write(chunk[:n])

			if cntWriteErr != nil {
				panic(cntWriteErr)
			}

			if cntWriteNum != n {
				panic(fmt.Sprint("Trying to write ", n, "but only ", cntWriteNum, "was written"))
			}

			if size == contents.Len() {

				importedCrc := md5.Sum(contents.Bytes())

				if fmt.Sprintf("%x", importedCrc) != crc {
					c.send <- "1"
					break
				}

				writeFiles <- WriteJob{c: c, file: importing, buffer: contents}

				if verbose {

					if contents.Len() != size {
						fmt.Print("Failed:")
					} else {
						fmt.Print("Success:")
					}

					fmt.Print(importing)

					fmt.Print("\n")
				}

				importing = ""
				contents.Reset()
				c.send <- OK
				continue
			}

			continue //Stay in file reading
		}

		message, err := reader.ReadBytes('\n')

		if err != nil {
			fmt.Println(err.Error())
			break
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
			crc = impinfo[2]

			contents.Reset()

			if verbose {
				fmt.Println("Import:", importing, "  Size:", size, "  Crc:", crc)
			}

			c.send <- OK
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
				log.Println("Fail to send data:", message)
				return
			}

			if message == "FINISH" {
				h.unregister <- c
				return
			}
		}
	}
}
