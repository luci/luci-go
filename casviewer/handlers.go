// Copyright 2020 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package casviewer

import (
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache) {
	mw := router.MiddlewareChain{}
	mw.Extend(
		withClientCacheMW(cc),
	)

	r.GET("/", mw, rootHanlder)
	r.GET("/projects/:proj/instances/:inst/blobs/:hash/:size/tree", mw, treeHandler)
	r.GET("/projects/:proj/instances/:inst/blobs/:hash/:size/", mw, getHandler)
}

func rootHanlder(c *router.Context) {
	// TODO(crbug.com/1121471): Add top page.
	logging.Debugf(c.Context, "Hello world")
	c.Writer.Write([]byte("Hello, world. This is CAS Viewer."))
}

func treeHandler(c *router.Context) {
	// TODO(crbug.com/1121471): implement me.
}

func getHandler(c *router.Context) {
	// TODO(crbug.com/1121471): implement me.
}
