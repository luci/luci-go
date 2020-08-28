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
	"fmt"
	"net/http"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache) {
	// TODO(crbug.com/1121471): Authorize request.
	baseMW := router.MiddlewareChain{}
	blobMW := baseMW.Extend(
		withClientCacheMW(cc),
	)

	r.GET("/", baseMW, rootHanlder)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size/tree", blobMW, treeHandler)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size", blobMW, getHandler)
}

func rootHanlder(c *router.Context) {
	// TODO(crbug.com/1121471): Add top page.
	logging.Debugf(c.Context, "Hello world")
	c.Writer.Write([]byte("Hello, world. This is CAS Viewer."))
}

// renderNotFound renders 400 BadRequest page.
func renderBadRequest(c *router.Context, errMsg string) {
	// TODO(crbug.com/1121471): render 404 html.
	m := fmt.Sprintf("Error: Bad Request. %s", errMsg)
	http.Error(c.Writer, m, http.StatusBadRequest)
}

// renderNotFound renders 404 NotFound page.
func renderNotFound(c *router.Context) {
	// TODO(crbug.com/1121471): render 400 html.
	http.Error(c.Writer, "Error: Not Found", http.StatusNotFound)
}

// renderInternalServerError renders 500 InternalServerError page.
func renderInternalServerError(c *router.Context, errMsg string) {
	// TODO(crbug.com/1121471): render 500 html.
	m := fmt.Sprintf("Error: %s", errMsg)
	http.Error(c.Writer, m, http.StatusInternalServerError)
}
