#!/bin/bash
set -e

a=$1
b=$2

go run main.go $1 > a.cfg
go run main.go $2 > b.cfg
git diff -U30 --no-index a.cfg b.cfg
