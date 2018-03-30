# Copyright 2017 The LUCI Authors. All rights reserved.
# Use of this source code is governed under the Apache License, Version 2.0
# that can be found in the LICENSE file.

projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

up:
	gae.py upload -A luci-notify

switch:
	gae.py switch -A luci-notify

up-dev:
	gae.py upload -A luci-notify-dev

switch-dev:
	gae.py switch -A luci-notify-dev

test:
	go test ./...

# Used mostly for .proto files.
generate:
	go generate ./...
