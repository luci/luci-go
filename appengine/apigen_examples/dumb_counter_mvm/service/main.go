// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/luci/luci-go/appengine/apigen_examples/dumb_counter/dumbCounter"
	"github.com/luci/luci-go/appengine/ephelper"
	"github.com/luci/luci-go/appengine/ephelper/epfrontend"
	"google.golang.org/appengine"
)

func main() {
	epfe := epfrontend.New("/api/", nil)
	h := ephelper.Helper{
		Frontend: epfe,
	}
	h.Register(endpoints.DefaultServer, dumbCounter.Example{}, dumbCounter.ServiceInfo, dumbCounter.MethodInfoMap)

	endpoints.HandleHTTP()
	epfe.HandleHTTP(nil)
	appengine.Main()
}
