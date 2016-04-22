export GOPATH=/var/tmp/gohome

install:
	go get gopkg.in/ini.v1

test:
	go test *.go

run:
	env DEBUG=1 go run *.go --config=config.sample.ini --verbose

build:
	go build -o ./wptree *.go;

release:
	/bin/bash -x release.sh $(BINPATH)

release-test:
	env GOOS=linux GOARCH=386 go build -o wpi *.go
	chmod +x wpi
	scp -i ~/SSH\ Keys/wpide wpi root@ams8.wpide.net:/usr/local/bin