#!/usr/bin/env bash

SCRIPT_DIR=$( cd -- "$( dirname -- "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )
cd ${SCRIPT_DIR}

mv go.mod.template go.mod
mv go.sum.template go.sum

go get -tool google.golang.org/protobuf/cmd/protoc-gen-go@latest
go mod tidy

mv go.mod go.mod.template
mv go.sum go.sum.template
