package main

import (
	"bytes"
	"crypto/md5"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
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
		rand.Seed(time.Now().UTC().UnixNano())
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

	wsEndpoint := "https://ws.wpide.net/event"

	if DEBUG {
		wsEndpoint = "http://localhost:9990/event"
	}

	jsonStr, err := json.Marshal(ev)

	if err != nil {
		fmt.Println("ERROR: Fail to marshal event:", err.Error())
		panic(err)
	}

	fmt.Println("Notify socket on:", wsEndpoint, "with data:", string(jsonStr))

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}

	req, err := http.NewRequest("POST", wsEndpoint, bytes.NewBuffer(jsonStr))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Transport: tr}
	resp, err := client.Do(req)

	if err != nil {
		fmt.Println("ERROR: Fail to notify socket:", err.Error())
		return
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

	dockerMysqlHost = strings.Trim(string(host), "\n")

	log.Println("Set docker host to:", dockerMysqlHost)
}

func WatchMysqlInstance() {
	for {
		log.Println("Check if docker mysql is on")
		checkMysqlIsUp()
		time.Sleep(time.Minute * 10)
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

	log.Println("Replace", url, "with", pURL)

	dbsNew := strings.Replace(dbs.String(), url, pURL, -1)

	if errWrite := ioutil.WriteFile(sqlFile, []byte(dbsNew), 0777); errWrite != nil {
		log.Println("Fail to write change sql file")
		return errWrite
	}

	dbs.Reset() // clear the buffer

	if err != nil {
		log.Println("Fail to read db buffer")
		panic(err)
	}

	if !Exists(sqlFile) {
		return errors.New(concat("SQL file missing from ", sqlFile))
	}

	checkMysqlIsUp()

	impcmd := concats("mysql", "-h", dockerMysqlHost, "-u", docker.DatabaseName, concat("-p", docker.DatabasePass), docker.DatabaseName, "<", sqlFile)

	shfile := concat(path, "/db-import.sh")

	if errWriteSh := ioutil.WriteFile(shfile, []byte(impcmd), 0755); errWriteSh != nil {
		log.Println("Fail to write sh file")
		return errWriteSh
	}

	if verbose {
		log.Println("Execute: /bin/bash", shfile)
	}

	errImportSql := exec.Command("/bin/bash", shfile).Run()

	if errImportSql != nil {
		log.Println("ERROR: Import failed ", errImportSql.Error())
		return errImportSql
	}

	errUpdateCfg := updateWordPressConfigFile(c, docker, path)
	if errUpdateCfg != nil {
		log.Println("ERROR: Fail to update config file:", errUpdateCfg.Error())
		return errUpdateCfg
	}

	return nil
}

func updateWordPressConfigFile(c *connection, docker *DockerResponse, path string) error {

	wpconfigPath := concat(path, "/wp-config.php")

	if !Exists(wpconfigPath) {
		return errors.New("wp-config.php file missing")
	}

	err := exec.Command("sed", "-i", "/DB_HOST/s/'[^']*'/'mysql'/2", wpconfigPath).Run()
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

	log.Println("Notify https://api.wpide.net/v1/import...")
	log.Println(" -> Send data: ", string(jsonStr))
	log.Println(" -> Authorization: ", c.access.Token)

	req, err := http.NewRequest("POST", "https://api.wpide.net/v1/import", bytes.NewBuffer(jsonStr))
	req.Header.Set("Authorization", c.access.Token)
	req.Header.Set("Content-Type", "application/json")

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}
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
			log.Println("ERROR: Response body is :", string(contents))
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

	log.Println("Removing ", mountsDir, "...")
	if err := RemoveDirContents(mountsDir); err != nil {
		if verbose {
			log.Println("Remove docker generated WordPress files for", pURL)
		}
		destroyDocker(instanceID, docker)
		log.Println(err.Error())
		return fin
	}

	log.Println("Copy from ", tempDir, "to", mountsDir, "...")
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
		fin.Error = "Fail to update new copy"
		return fin
	}

	if errNotify := notifyDashboard(c, docker, instanceID, wpAdminEmail, pURL); errNotify != nil {
		destroyDocker(instanceID, docker)
		log.Println(errNotify.Error())
		fin.Error = "Fail to save info about import"
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

	if !Exists(writePathBuffer.String()) {
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

	if !Exists(path.Dir(filePath.String())) {
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

func concats(strs ...string) string {
	var buffer bytes.Buffer
	var size int = len(strs)
	for i, str := range strs {
		buffer.WriteString(str)
		if i < size-1 {
			buffer.WriteString(" ")
		}
	}
	return buffer.String()
}
