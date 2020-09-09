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

	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"
)

var permMintToken = realms.RegisterPermission("luci.serviceAccounts.mintToken")

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
	switch ok, err := auth.HasPermission(c.Context, permMintToken, readOnlyRealm(c.Params)); {
	case err != nil:
		renderErrorPage(c.Context, c.Writer, err)
	case !ok:
		err = errors.New("permission denied", grpcutil.PermissionDeniedTag)
		renderErrorPage(c.Context, c.Writer, err)
	default:
		next(c)
	}
}

func rootHanlder(c *router.Context) {
	// TODO(crbug.com/1121471): Add top page.
	logging.Debugf(c.Context, "Hello world")
	hello := fmt.Sprintf(
		"Hello, world. This is CAS Viewer. Identity: %v", auth.CurrentIdentity(c.Context))
	c.Writer.Write([]byte(hello))
}

// readOnlyRealm constructs a read-only realm name corresponding to the project in the URL params.
func readOnlyRealm(p httprouter.Params) string {
	return fmt.Sprintf("@internal:%s/cas-read-only", p.ByName("project"))
}

// renderErrorPage renders an appropriate error page.
func renderErrorPage(ctx context.Context, w http.ResponseWriter, err error) {
	errors.Log(ctx, err)
	// TODO(crbug.com/1121471): render error page.
	statusCode := grpcutil.CodeStatus(grpcutil.Code(err))
	http.Error(w, fmt.Sprintf("Error: %s", err.Error()), statusCode)
}
