// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package module

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/tumble"

	// Include mutations package so its Mutations will register with tumble via
	// init().
	_ "github.com/luci/luci-go/appengine/logdog/coordinator/mutations"
)

func init() {
	tmb := tumble.Service{
		Middleware: func(c context.Context) context.Context {
			if err := config.UseConfig(&c); err != nil {
				panic(err)
			}
			c = coordinator.UseProdServices(c)
			return c
		},
	}

	router := httprouter.New()
	tmb.InstallHandlers(router)

	http.Handle("/", router)
}
