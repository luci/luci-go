// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/cmd/dm/deps"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/server/discovery"
	"github.com/luci/luci-go/server/prpc"
)

var myTumble = tumble.DefaultConfig()

func init() {
	router := httprouter.New()
	svr := prpc.Server{}
	deps.RegisterDepsServer(&svr)
	discovery.Enable(&svr)

	myTumble.InstallHandlers(router)
	http.Handle("/", router)
}
