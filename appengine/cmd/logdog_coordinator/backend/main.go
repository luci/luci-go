// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package module

import (
	"net/http"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	"github.com/luci/luci-go/appengine/tumble"
	"github.com/luci/luci-go/server/router"

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

	r := router.New()
	tmb.InstallHandlers(r)

	http.Handle("/", r)
}
