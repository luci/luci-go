# Copyright 2016 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.
#
# This is a simple Makefile to build "dist" versions for all web app
# subdirectories using gulp.

APPS = $(patsubst %/gulpfile.js,%,$(wildcard */gulpfile.js))

.PHONY: all
all: $(APPS)

.PHONY: $(APPS)
$(APPS):
	cd "$@" && gulp
