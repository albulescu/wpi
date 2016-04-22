#!/bin/bash

set +x

BINPATH=$1
APPNAME="wpi"

if [ ! -d "$BINPATH" ]; then
    echo "Directory for deploying $BINPATH does not exist"
    exit 1
fi

VERSION=$(/usr/bin/git describe --abbrev=0 --tags | cut -d'v' -f 2)

if [ -f "$BINPATH/$APPNAME" ]; then
    LATEST=$($BINPATH/$APPNAME --version)
else
    LATEST="0.0.0"
fi

if [ -n "$VERSION" ] && [ "$VERSION" != "$LATEST" ]; then

    git checkout "$VERSION"

    go build -ldflags "-X main.VERSION=`echo $VERSION`" -o "./$APPNAME" *.go

    sudo cp "./$APPNAME" $BINPATH

    echo "Release version $(./$APPNAME --version)"

    rm -rf "./$APPNAME"
else
    echo "No new releases. Latest version is $LATEST"
fi