# Copyright 2014 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

mkfile_path := $(abspath $(lastword $(MAKEFILE_LIST)))
projdir := $(patsubst %/,%,$(dir $(mkfile_path)))

dev:
	$(projdir)/../../../../../../../../luci/appengine/components/tools/gae.py devserver --app-dir $(projdir) -- --host 0.0.0.0 --port 8081
