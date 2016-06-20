# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.
#
# This is a simple Makefile to build "dist" versions for all web app
# subdirectories using gulp.

ROOT = $(PWD)
APPS_DIR = $(ROOT)/apps

.PHONY: all
all: apps

.PHONY: apps
apps:
	make -C $(APPS_DIR) all

# The setup target will install all npm and bower prerequisites.
.PHONY: setup
setup:
	npm install
	bower install
