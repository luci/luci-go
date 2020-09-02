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
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/client"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/julienschmidt/httprouter"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/router"
	"google.golang.org/grpc/status"
)

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache, authMW router.Middleware) {
	baseMW := router.NewMiddlewareChain(
		authMW,
	)
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
	hello := fmt.Sprintf(
		"Hello, world. This is CAS Viewer. Identity: %v", auth.CurrentIdentity(c.Context))
	c.Writer.Write([]byte(hello))
}

func treeHandler(c *router.Context) {
	cl, bd, err := prepcoessBlobRequest(c)
	if err != nil {
		return
	}
	renderTree(c.Context, c.Writer, cl, bd)
}

func getHandler(c *router.Context) {
	cl, bd, err := prepcoessBlobRequest(c)
	if err != nil {
		return
	}
	returnBlob(c.Context, c.Writer, cl, bd)
}

// prepcoessBlobRequest returns Client and Digest for the requested blob.
func prepcoessBlobRequest(c *router.Context) (*client.Client, *digest.Digest, error) {
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	if err != nil {
		logging.Errorf(c.Context, "failed to get CAS client: %s", err)
		renderInternalServerError(c.Context, c.Writer, status.Code(err).String())
		return nil, nil, err
	}
	d, err := blobDigest(c.Params)
	if err != nil {
		renderBadRequest(c.Context, c.Writer, "Digest size must be number")
		return nil, nil, err
	}
	return cl, d, nil
}

// fullInstName constructs full instance name from the URL parameters.
func fullInstName(p httprouter.Params) string {
	return fmt.Sprintf(
		"projects/%s/instances/%s", p.ByName("project"), p.ByName("instance"))
}

// blobDigest constructs a Digest from the URL parameters.
func blobDigest(p httprouter.Params) (*digest.Digest, error) {
	size, err := strconv.ParseInt(p.ByName("size"), 10, 64)
	if err != nil {
		return nil, err
	}

	d := &digest.Digest{
		Hash: p.ByName("hash"),
		Size: size,
	}
	return d, nil
}

// renderBadRequest renders 400 BadRequest page.
func renderBadRequest(ctx context.Context, w http.ResponseWriter, errMsg string) {
	// TODO(crbug.com/1121471): render 400 html.
	m := fmt.Sprintf("Error: Bad Request. %s", errMsg)
	http.Error(w, m, http.StatusBadRequest)
}

// renderNotFound renders 404 NotFound page.
func renderNotFound(ctx context.Context, w http.ResponseWriter) {
	// TODO(crbug.com/1121471): render 404 html.
	http.Error(w, "Error: Not Found", http.StatusNotFound)
}

// renderInternalServerError renders 500 InternalServerError page.
func renderInternalServerError(ctx context.Context, w http.ResponseWriter, errMsg string) {
	// TODO(crbug.com/1121471): render 500 html.
	m := fmt.Sprintf("Error: %s", errMsg)
	http.Error(w, m, http.StatusInternalServerError)
}
