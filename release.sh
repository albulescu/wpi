#!/bin/bash

set +x

BINPATH=$1

if [ ! -d "$BINPATH" ]; then
    echo "Directory for deploying $BINPATH does not exist"
    exit 1
fi

VERSION=$(/usr/bin/git describe --abbrev=0 --tags | cut -d'v' -f 2)

if [ -f "$BINPATH/wpi" ]; then
    LATEST=$($BINPATH/wpi --version)
else
    LATEST="0.0.0"
fi

if [ -n "$VERSION" ] && [ "$VERSION" != "$LATEST" ]; then

    git checkout "$VERSION"

    go build -ldflags "-X main.VERSION=`echo $VERSION`" -o ./wpi *.go

    sudo cp ./wpi $BINPATH

    echo "Release version $(./wpi --version)"

    rm -rf ./wpi
else
    echo "No new releases. Latest version is $LATEST"
fi