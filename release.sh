#!/bin/bash

set +x

if [ ! -f "/etc/init.d/wpi" ]; then
    echo "Install service file"
    sudo cp wpi.service /etc/init.d/wpi
    sudo chmod +x /etc/init.d/wpi
fi

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

    sudo /etc/init.d/wpi stop

    sudo cp "./$APPNAME" $BINPATH

    echo "Release version $(./$APPNAME --version)"

    rm -rf "./$APPNAME"

    sudo /etc/init.d/wpi start

else
    echo "No new releases. Latest version is $LATEST"
fi