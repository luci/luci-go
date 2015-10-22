// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	_ "github.com/luci/luci-go/appengine/apigen_examples/dumb_counter/service"
	"google.golang.org/appengine"
)

func main() {
	appengine.Main()
}
