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

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache) {
	baseMW := router.NewMiddlewareChain()
	blobMW := baseMW.Extend(
		checkPermission,
		withClientCacheMW(cc),
	)

	r.GET("/", baseMW, rootHanlder)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size/tree", blobMW, treeHandler)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size", blobMW, getHandler)
}

// checkPermission checks if the user has permission to read the blob.
func checkPermission(c *router.Context, next router.Handler) {
	ctx := c.Context
	authDB, err := auth.GetDB(ctx)
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	ok, err := authDB.HasPermission(
		ctx,
		auth.CurrentIdentity(ctx),
		realms.RegisterPermission("luci.serviceAccounts.mintToken"),
		readOnlyRealm(c.Params))
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	if !ok {
		err = errors.New("permission denied", grpcutil.PermissionDeniedTag)
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	next(c)
}

func rootHanlder(c *router.Context) {
	// TODO(crbug.com/1121471): Add top page.
	logging.Debugf(c.Context, "Hello world")
	hello := fmt.Sprintf(
		"Hello, world. This is CAS Viewer. Identity: %v", auth.CurrentIdentity(c.Context))
	c.Writer.Write([]byte(hello))
}

func treeHandler(c *router.Context) {
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	bd, err := blobDigest(c.Params)
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	err = renderTree(c.Context, c.Writer, cl, bd)
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
	}
}

func getHandler(c *router.Context) {
	cl, err := GetClient(c.Context, fullInstName(c.Params))
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	bd, err := blobDigest(c.Params)
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
		return
	}
	err = returnBlob(c.Context, c.Writer, cl, bd)
	if err != nil {
		renderErrorPage(c.Context, c.Writer, err)
	}
}

func readOnlyRealm(p httprouter.Params) string {
	return fmt.Sprintf("@internal:%s/cas-read-only", p.ByName("project"))
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
		err = errors.Annotate(err, "Digest size must be number").Tag(grpcutil.InvalidArgumentTag).Err()
		return nil, err
	}

	return &digest.Digest{
		Hash: p.ByName("hash"),
		Size: size,
	}, nil
}

// renderErrorPage renders an appropriate error page.
func renderErrorPage(ctx context.Context, w http.ResponseWriter, err error) {
	errors.Log(ctx, err)
	// TODO(crbug.com/1121471): render error page.
	statusCode := grpcutil.CodeStatus(grpcutil.Code(err))
	http.Error(w, fmt.Sprintf("Error: %s", err.Error()), statusCode)
}
