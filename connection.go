package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dvsekhvalnov/jose2go"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"time"
)

/**
 * TODO:
- Ping sockets
- Create docker & Create database
- Remove old files from mounts && Copy files to mounts
- Replace url's
- Update wp-config.php
- Ping sockets
- Cleanup temp

- fail
- Delete docker instance ( use /commands )
- Ping sockets
- Inform plugin


CALL IMPORT  https://api.wpide.net/vi/import

Add Header:
Authorization: Bearer ...

{
  "domain":"",
  "title":"",
  "adminEmail":"",
  "container_id":""$CONTAINER_ID"",
  "database_name":""$DATABASE_NAME"",
  "database_pass":""$DATABASE_PASS"",
  "container_port":""$PORT""
  "server_ip":""
}

*/

var OK string = "0"
var CHUNK_SIZE = 1024

type AccessToken struct {
	Scope    string `json:"scope,omitempty"`
	Data     string `json:"data,omitempty"`
	IssuedAt int64  `json:"iat,omitempty"`
	ExpireAt int64  `json:"exp,omitempty"`
	Token    string
}

func (at *AccessToken) String() string {
	return concat("[Scope:", at.Scope, ", Data:", fmt.Sprintf("%v", at.Data), "]")
}

type connection struct {
	conn   net.Conn
	send   chan string
	params map[string]string
	access *AccessToken
}

func (c *connection) String() string {
	return c.conn.RemoteAddr().String()
}

func (c *connection) auth(token string) bool {

	fmt.Println("Auth with:", token)

	payload, _, err := jose.Decode(token, []byte(config.Secret))

	if err != nil {
		c.conn.Write([]byte("1\n"))
		c.conn.Close()
		return false
	}

	jwt := new(AccessToken)

	jwt.Token = token

	err = json.Unmarshal([]byte(payload), &jwt)

	if err != nil {
		panic(err)
	}

	expire := jwt.ExpireAt - int64(time.Now().Unix())

	if expire < 0 {
		fmt.Println("Token expired")
		c.conn.Write([]byte("2\n"))
		c.conn.Close()
		return false
	}

	if jwt.Scope != "import" {
		fmt.Println("Token has other scope than import")
		c.conn.Write([]byte("3\n"))
		c.conn.Close()
		return false
	}

	fmt.Println("Auth OK")
	fmt.Println("Auth Access:", jwt.String())
	c.access = jwt

	c.send <- "0"

	h.register <- c

	return true

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

	if part[0] == "url" {
		sendSocketEvent("import.start", map[string]interface{}{
			"user": c.access.Data,
			"url":  part[1],
		})
	}

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

		if command != "AUTH" && c.access == nil {
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

			resp := finish(c)

			if resp.Success {
				sendSocketEvent("import.complete", map[string]interface{}{
					"user": c.access.Data,
					"url":  resp.Url,
				})
			} else {
				sendSocketEvent("import.error", map[string]interface{}{
					"user":  c.access.Data,
					"error": resp.Error,
				})
			}

			json, err := json.Marshal(resp)

			if err != nil {
				panic(err)
			}

			c.conn.Write(json)
			c.conn.Close()
			return

		} else if command == "IMPORT" {

			impinfo := strings.Split(param, "|")

			cImportFile = impinfo[0]
			cImportSize, _ = strconv.Atoi(impinfo[1])
			cImportCRC = impinfo[2]

			filePath := prepareFilePath(c, cImportFile)

			if verbose {
				log.Println("Import:", cImportFile, "  Size:", cImportSize, "  Crc:", cImportCRC)
			}

			cFileBuffer.Reset()

			// -- CHECK IF FILE EXISTS TO SKIP IT -----

			if Exists(filePath) {

				filePathCrc, err := FileCrc(filePath)
				if err != nil {
					panic(err)
				}

				if filePathCrc == cImportCRC {
					log.Println("File already exist:", cImportFile)
					c.conn.Write([]byte("2\n"))
					cImportFile = ""
					continue
				}
			}

			// -----------------------------------------

			/**
			 * If file size of imported file is empty just
			 * create an empty file in destination path
			 */

			if cImportSize == 0 {

				if verbose {
					fmt.Println("File is empty. Just create the file!")
				}

				err := createEmptyFile(filePath)

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
