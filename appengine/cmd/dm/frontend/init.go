// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/GoogleCloudPlatform/go-endpoints/endpoints"
	"github.com/julienschmidt/httprouter"
	dm "github.com/luci/luci-go/appengine/cmd/dm/service"
	"github.com/luci/luci-go/appengine/tumble"
)

var myTumble = tumble.DefaultConfig()

func init() {
	srv := endpoints.NewServer("")
	err := dm.RegisterEndpointsService(srv)
	if err != nil {
		panic(err)
	}
	http.Handle("/_ah/spi/", srv)

	router := httprouter.New()
	myTumble.InstallHandlers(router)
	http.Handle("/", router)
}
