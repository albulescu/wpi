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

func (c *connection) auth(token string) bool {

	fmt.Println("Auth with:", token)

	if token == "abc" {
		c.token = token
		fmt.Println("Auth OK")
		c.send <- "0"
		h.register <- c
		return true
	} else {
		fmt.Println("Auth FAIL")
		c.conn.Write([]byte("1\n"))
		c.conn.Close()
		return false
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
	c.conn.Write([]byte(message))
	c.conn.Write([]byte("\n"))
	c.conn.Close()
}

func (c *connection) readPump() {

	var cImportFile string
	var cImportSize int
	var cImportCRC string
	var cFileBuffer bytes.Buffer

	reader := bufio.NewReaderSize(c.conn, CHUNK_SIZE)

	for {

		if cImportFile != "" {

			var chunk []byte = make([]byte, CHUNK_SIZE)

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

			cntWriteNum, cntWriteErr := cFileBuffer.Write(chunk[:n])

			if cntWriteErr != nil {
				panic(cntWriteErr)
			}

			if cntWriteNum != n {
				panic(fmt.Sprint("Trying to write ", n, "but only ", cntWriteNum, "was written"))
			}

			if cImportSize == cFileBuffer.Len() {

				importedCrc := md5.Sum(cFileBuffer.Bytes())

				if fmt.Sprintf("%x", importedCrc) != cImportCRC {
					c.send <- "1"
					break
				}

				writeFiles <- WriteJob{c: c, file: cImportFile, buffer: cFileBuffer}

				if verbose {

					if cFileBuffer.Len() != cImportSize {
						fmt.Print("Failed:")
					} else {
						fmt.Print("Success:")
					}

					fmt.Print(cImportFile)

					fmt.Print("\n")
				}

				cImportFile = ""
				cFileBuffer.Reset()
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
			if !c.auth(param) {
				return
			}
		} else if command == "SET" {
			c.set(param)
		} else if command == "FINISH" {
			if verbose {
				fmt.Println("Finished importing files")
			}
			c.conn.Write([]byte("{\"url\":\"http://wpide.net\"}"))
			c.conn.Close()
			return
		} else if command == "IMPORT" {

			impinfo := strings.Split(param, "|")

			cImportFile = impinfo[0]
			cImportSize, _ = strconv.Atoi(impinfo[1])
			cImportCRC = impinfo[2]

			if verbose {
				fmt.Println("Import:", cImportFile, "  Size:", cImportSize, "  Crc:", cImportCRC)
			}

			cFileBuffer.Reset()

			if cImportSize == 0 {

				if verbose {
					fmt.Println("File is empty. Just create the file!")
				}

				err := createEmptyFile(prepareFilePath(c, cImportFile))

				if err != nil {
					panic(err)
				}

				//Write ok for import
				//packet and for data transfer ok
				c.conn.Write([]byte("0\n0\n"))

				cImportFile = ""

				continue
			}

			c.send <- OK
		} else {
			fmt.Println("Unknown command", command)
		}
	}
}

func (c *connection) writePump() {

	defer func() {
		fmt.Println("Defered closing write pump")
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
