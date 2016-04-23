package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

/*
{
  "container_id":""$CONTAINER_ID"",
  "database_name":""$DATABASE_NAME"",
  "database_pass":""$DATABASE_PASS"",
  "port":""$PORT""
}
*/

type FinishResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
	Url     string `json:"url,omitempty"`
}

type DockerResponse struct {
	ContainerID  string `json:"container_id"`
	DatabaseName string `json:"database_name"`
	DatabasePass string `json:"database_pass"`
	Port         string `json:"port"`
}

type Event struct {
	Name string                 `json:"event"`
	Data map[string]interface{} `json:"data,omitempty"`
}

func randomID() string {
	time := strconv.FormatInt(time.Now().Unix(), 10)
	h := md5.New()
	io.WriteString(h, time)
	return fmt.Sprintf("%x", h.Sum(nil))
}

func randomDomain() string {
	n := 5
	const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	b := make([]byte, n)
	for i := range b {
		b[i] = letterBytes[rand.Intn(len(letterBytes))]
	}

	return concat("dev-i", string(b))
}

func destroyDocker(id string, docker *DockerResponse) {
	log.Println(concat("Deleting ", id, " docker instance..."))
	exec.Command("/commands/drop_instance.sh", id, docker.DatabaseName).Run()
	log.Println(concat("Deleting ", id, " docker instance complete!"))
}

func createDockerInstance(c *connection, wpTitle string, wpAdminEmail string, instanceID string, pURL string) (*DockerResponse, error) {

	wpAdminUser := ""
	wpAdminPass := ""

	cmd := exec.Command("/commands/create_instance.sh", wpTitle, wpAdminUser, wpAdminPass, wpAdminEmail, instanceID, pURL)

	dr := &DockerResponse{}

	jsonString, err := cmd.Output()

	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(jsonString, dr); err != nil {
		return nil, err
	}

	return dr, nil
}

func sendSocketEvent(event string, data map[string]interface{}) {

	ev := &Event{
		event,
		data,
	}

	wsEndpoint := "http://ws.wpide.net/event"

	if DEBUG {
		wsEndpoint = "http://localhost:9990/event"
	}

	jsonStr, err := json.Marshal(ev)

	if err != nil {
		panic(err)
	}

	fmt.Println("Notify socket on:", wsEndpoint, "with data:", string(jsonStr))

	req, err := http.NewRequest("POST", wsEndpoint, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("ERROR: Fail to notify socket")
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Println("ERROR: Response code is ", strconv.Itoa(resp.StatusCode))
	}
}

func GetLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

var dockerMysqlHost string

func checkMysqlIsUp() {

	err := exec.Command("docker ps | grep wpide-mysql").Run()

	if err != nil {
		dockerMysqlHost = ""
		log.Println("Starting docker mysql instance...")
		exec.Command("docker", "start", "wpide-mysql").Run()
		log.Println("Docker mysql is ready!")
	}

	if dockerMysqlHost != "" {
		return
	}

	host, err := exec.Command("docker", "inspect", "--format", "{{ .NetworkSettings.IPAddress }}", "wpide-mysql").Output()

	if err != nil {
		log.Println("Fail to get mysql host")
		panic(err)
	}

	dockerMysqlHost = string(host)
	log.Println("Set docker host to:", dockerMysqlHost)
}

func WatchMysqlInstance() {
	for {
		log.Println("Check if docker mysql is on")
		checkMysqlIsUp()
		time.Sleep(time.Minute)
	}
}

func updateWordPress(c *connection, docker *DockerResponse, path string, pURL string) error {

	sqlFile := concat(path, "/db.sql")

	dbs, err := ReadFile(sqlFile)

	if err != nil {
		log.Println("Fail to read sql file:", sqlFile)
		return err
	}

	url, err := c.get("url")

	if err != nil {
		log.Println("Fail to get url from connection")
		return err
	}

	dbsNew := strings.Replace(dbs.String(), url, pURL, -1)

	dbs.Reset() // clear the buffer

	dbbuf := bufio.NewReader(strings.NewReader(dbsNew))

	if err != nil {
		log.Println("Fail to read db buffer")
		panic(err)
	}

	checkMysqlIsUp()

	mysqlHostString := strings.Trim(string(dockerMysqlHost), "\n")

	connectInfo := concat(docker.DatabaseName, ":", docker.DatabasePass, "@tcp(", mysqlHostString, ":3306)/", docker.DatabaseName)

	log.Println("Connecting to mysql:", connectInfo)

	db, err := sql.Open("mysql", connectInfo)
	defer db.Close()

	// CLEAN DATABASE
	//
	result, err := db.Query("show tables")

	if err != nil {
		log.Println("Fail to get tables:", err.Error())
		return err
	}

	for result.Next() {

		var tableName string

		result.Scan(&tableName)

		_, err := db.Exec(concat("DROP TABLE IF EXISTS ", tableName))

		if err != nil {
			log.Println("Fail to drop table :", err.Error())
		}
	}

	if err != nil {
		panic(err)
	}

	if err != nil {
		fmt.Println("Failed to connect to docker mysql:", connectInfo)
		return err
	}

	for {

		sqlString, err := dbbuf.ReadBytes('\n')
		sqlString = sqlString[:len(sqlString)]

		if err == io.EOF {
			break
		}

		if len(sqlString) == 0 {
			log.Println("Sql is empty. Skip!")
			continue
		}
		_, err = db.Exec(string(sqlString))

		if err != nil {
			fmt.Println("Fail to execute query:", string(sqlString))
			return err
		}

	}

	err = updateWordPressConfigFile(c, docker, path)

	if err != nil {
		fmt.Println("ERROR: ", err.Error())
		return err
	}

	return nil
}

func updateWordPressConfigFile(c *connection, docker *DockerResponse, path string) error {

	wpconfigPath := concat(path, "/wp-config.php")

	wpconfExists, err := Exists(wpconfigPath)

	if err != nil {
		panic(err)
		return err
	}

	if !wpconfExists {
		return errors.New("wp-config.php file missing")
	}

	err = exec.Command("sed", "-i", "/DB_HOST/s/'[^']*'/'mysql'/2", wpconfigPath).Run()
	if err != nil {
		return err
	}

	err = exec.Command("sed", "-i", concat("/DB_NAME/s/'[^']*'/'", docker.DatabaseName, "'/2"), wpconfigPath).Run()
	if err != nil {
		return err
	}

	err = exec.Command("sed", "-i", concat("/DB_USER/s/'[^']*'/'", docker.DatabaseName, "'/2"), wpconfigPath).Run()
	if err != nil {
		return err
	}

	err = exec.Command("sed", "-i", concat("/DB_PASSWORD/s/'[^']*'/'", docker.DatabasePass, "'/2"), wpconfigPath).Run()
	if err != nil {
		return err
	}

	return nil
}

func notifyDashboard(c *connection, docker *DockerResponse, instance string, wpAdminEmail string, pURL string) error {

	wpTitle, err := c.get("name")

	if err != nil {
		panic(err)
	}

	data := map[string]interface{}{
		"domain":         pURL,
		"title":          wpTitle,
		"adminEmail":     wpAdminEmail,
		"container_id":   docker.ContainerID,
		"database_name":  docker.DatabaseName,
		"database_pass":  docker.DatabasePass,
		"container_port": docker.Port,
		"server_ip":      GetLocalIP(),
	}

	jsonStr, err := json.Marshal(data)

	if err != nil {
		return err
	}

	log.Println("Notify dashboard:", string(jsonStr))

	req, err := http.NewRequest("POST", "https://api.wpide.net/v1/import", bytes.NewBuffer(jsonStr))
	req.Header.Set("Authorization", c.access.Token)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		return err
	}

	defer resp.Body.Close()

	if resp.StatusCode != 200 {

		log.Println("ERROR: Response code is :", strconv.Itoa(resp.StatusCode))

		contents, err := ioutil.ReadAll(resp.Body)

		if err != nil {
			fmt.Printf("%s", err)
		} else {
			log.Println("ERROR: Response code is :", string(contents))
		}

		return errors.New("Fail to properly notify dashboard.")
	}

	return nil
}

func finish(c *connection) *FinishResponse {

	instanceID := randomID()

	wpTitle, err := c.get("name")

	if err != nil {
		panic(err)
	}

	wpAdminEmail, err := c.get("admin_email")

	if err != nil {
		panic(err)
	}

	host := getHostFromURL(c)
	subDomain := strings.Replace(host, ".", "-", -1)
	pURL := concat("http://", subDomain, ".wpide.net")

	if host == "localhost" {
		pURL = concat("http://", randomDomain(), ".wpide.net")
	}

	if verbose {
		log.Println("Create docker instance for", pURL)
	}

	docker, err := createDockerInstance(c, wpTitle, wpAdminEmail, instanceID, pURL)

	fin := &FinishResponse{}

	if err != nil {
		fin.Success = false
		fin.Error = "Fail to create docker instance"
		log.Println("Fail to create docker instance", err.Error())
		return fin
	}

	tempDir := getImportTempPath(c)
	mountsDir := concat(config.Mounts, "/", instanceID)

	if err := RemoveDirContents(mountsDir); err != nil {
		if verbose {
			log.Println("Remove docker generated WordPress files for", pURL)
		}
		destroyDocker(instanceID, docker)
		log.Println(err.Error())
		return fin
	}

	if errCopy := CopyDir(tempDir, mountsDir); errCopy != nil {
		if verbose {
			log.Println("Copy files from temp to mounts for", pURL)
		}
		destroyDocker(instanceID, docker)
		log.Println(errCopy.Error())
		return fin
	}

	if err := updateWordPress(c, docker, mountsDir, pURL); err != nil {
		destroyDocker(instanceID, docker)
		log.Println(err.Error())
		return fin
	}

	if errNotify := notifyDashboard(c, docker, instanceID, wpAdminEmail, pURL); errNotify != nil {
		destroyDocker(instanceID, docker)
		log.Println(errNotify.Error())
		return fin
	}

	fin.Success = true
	fin.Url = pURL

	return fin
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

func getHostFromURL(c *connection) string {

	prop, err := c.get("url")

	if err != nil {
		panic(err)
	}

	u, err := url.Parse(prop)

	if err != nil {
		panic(err)
	}

	return strings.Split(u.Host, ":")[0]
}

func getImportTempPath(c *connection) string {

	host := getHostFromURL(c)

	var writePathBuffer bytes.Buffer

	tempPath, err := filepath.Abs(config.Temp)

	if err != nil {
		panic(err)
	}

	writePathBuffer.WriteString(tempPath)
	writePathBuffer.WriteString("/")
	writePathBuffer.WriteString(host)

	siteExist, err := Exists(writePathBuffer.String())

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

	return writePathBuffer.String()
}

func prepareFilePath(c *connection, file string) string {

	importPath := getImportTempPath(c)

	var filePath bytes.Buffer

	filePath.WriteString(importPath)
	filePath.WriteString("/")
	filePath.WriteString(file)

	fExists, err := Exists(path.Dir(filePath.String()))

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
