export GOPATH=/var/tmp/gohome

install:
	go get gopkg.in/ini.v1
	go get github.com/dvsekhvalnov/jose2go
	go get github.com/go-sql-driver/mysql

test:
	go test *.go

run:
	env DEBUG=1 go run *.go --config=config.sample.ini

build:
	go build -o ./wptree *.go;

release:
	/bin/bash -x release.sh $(BINPATH)
