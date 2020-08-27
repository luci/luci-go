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

	repb "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache) {
	// TODO(crbug.com/1121471): Authorize request.
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
	b, err := retrieveBlob(c)
	if err != nil {
		return
	}

	d := &repb.Directory{}
	if err = proto.Unmarshal(b, d); err != nil {
		http.Error(c.Writer, "Failed to unmarshal", http.StatusInternalServerError)
		return
	}

	// TODO(crbug.com/1121471): render html.
	dirs := d.GetDirectories()
	c.Writer.Write([]byte(fmt.Sprintf("dirs: %v", dirs)))

	files := d.GetFiles()
	c.Writer.Write([]byte(fmt.Sprintf("files: %v", files)))

	links := d.GetSymlinks()
	c.Writer.Write([]byte(fmt.Sprintf("symlinks %v", links)))
}

func getHandler(c *router.Context) {
	b, err := retrieveBlob(c)
	if err != nil {
		return
	}

	// TODO(crbug.com/1121471): return with appropriate headers.
	c.Writer.Write(b)
}

func retrieveBlob(c *router.Context) ([]byte, error) {
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	if err != nil {
		err = errors.Annotate(err, "failed to initialize CAS client").Err()
		http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
		return nil, err
	}

	b, err := cl.ReadBytes(c.Context, fullResourceName(c.Params))
	// TODO: NotFound returning here?
	if err != nil {
		err = errors.Annotate(err, "failed to read bytes").Err()
		http.Error(c.Writer, err.Error(), http.StatusInternalServerError)
		return nil, err
	}
	if b == nil {
		err = errors.Annotate(err, "Blob not found").Err()
		http.Error(c.Writer, "Not Found", http.StatusNotFound)
		return nil, err
	}
	return b, nil
}

func fullInstName(p httprouter.Params) string {
	return fmt.Sprintf("projects/%s/instances/%s", p.ByName(":proj"), p.ByName(":inst"))
}

func fullResourceName(p httprouter.Params) string {
	return fmt.Sprintf(
		"projects/%s/instances/%s/blobs/%s/%s",
		p.ByName(":proj"), p.ByName(":inst"), p.ByName(":hash"), p.ByName(":size"))
}
