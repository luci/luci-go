# Copyright 2020 The Chromium OS Authors. All rights reserved.                                                                                                                           
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

MKFILE_PATH := $(abspath $(lastword $(MAKEFILE_LIST)))
PROJECT_PATH := $(patsubst %/,%,$(dir $(MKFILE_PATH)))

define help_message
Helpers for managing ufs project in $(PROJECT_PATH):
  gen: regenerate all go bindings
endef

.PHONY: help
help:
	$(info $(help_message))

# Generate bindings
.PHONY: gen
gen:
	go generate ./...
