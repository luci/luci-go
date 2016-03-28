# Copyright 2016 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.
#
# This is a simple Makefile to build "dist" versions for all web app
# subdirectories using gulp.

APPS = $(patsubst %/gulpfile.js,%,$(wildcard */gulpfile.js))

.PHONY: all
all: $(APPS)

.PHONY: $(APPS)
$(APPS):
	cd "$@" && gulp
