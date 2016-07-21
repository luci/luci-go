# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.
#
# This is a simple Makefile to build "dist" versions for all web app
# subdirectories using gulp.

ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
GULP_OUT ?= $(shell dirname $(ROOT_DIR))

ALL_GULPFILES = $(wildcard $(ROOT_DIR)/*/gulpfile.js)
APPS = $(patsubst $(ROOT_DIR)/%/gulpfile.js,%,$(ALL_GULPFILES))

.PHONY: all
all: $(APPS)

.PHONY: $(APPS)
$(APPS):
	cd "$@" && gulp --out "$(GULP_OUT)"
