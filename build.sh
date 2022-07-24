#!/bin/bash

set -e

printf "\n-- $(date) -- Starting --\n"

[ -d build ] && rm -rf build
mkdir -p build/lib

go run build.go
tar xzf lib/go-sqlite3-*.tar.gz -C build/lib
mv build/lib/go-sqlite3-1.14.12 build/lib/go-sqlite3

printf "\n-- $(date) -- Building --\n"

# apt-get install gcc glibc-static
# yum install gcc glibc-static
GO111MODULE=off go build -ldflags="-s -w -linkmode=external -extldflags=-static" -o build/lnxmonsrv build/lnxmonsrv.go
GO111MODULE=off go build -ldflags="-s -w -linkmode=external -extldflags=-static" -o build/lnxmoncli build/lnxmoncli.go

[ -f build/lnxmonsrv.go ] && rm build/lnxmonsrv.go
[ -f build/lnxmoncli.go ] && rm build/lnxmoncli.go

[ -d build/lib ] && rm -rf build/lib

printf "\n-- $(date) -- Finished --\n"
