#!/bin/bash

echo Building lucicfg
go build -o lucicfg ../cmd/lucicfg

echo Running it
./lucicfg gen -stdlib ../stdlib LUCI.sky
