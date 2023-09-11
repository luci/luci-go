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
	"html/template"
	"net/http"
	"os"
	"strconv"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	"github.com/dustin/go-humanize"
	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// Path to the templates dir from the executable.
const templatePath = "templates"

var permMintToken = realms.RegisterPermission("luci.serviceAccounts.mintToken")

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache, appVersion string) {
	baseMW := router.NewMiddlewareChain(
		templates.WithTemplates(getTemplateBundle(appVersion)),
	)
	blobMW := baseMW.Extend(
		checkPermission,
		withClientCacheMW(cc),
	)

	r.GET("/", baseMW, rootHandler)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size/tree", blobMW, treeHandler)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size", blobMW, getHandler)
}

// getTemplateBundles returns template Bundle with base args.
func getTemplateBundle(appVersion string) *templates.Bundle {
	return &templates.Bundle{
		Loader:          templates.FileSystemLoader(os.DirFS(templatePath)),
		DefaultTemplate: "base",
		DefaultArgs: func(c context.Context, e *templates.Extra) (templates.Args, error) {
			return templates.Args{
				"AppVersion": appVersion,
				"User":       auth.CurrentUser(c),
			}, nil
		},
		FuncMap: template.FuncMap{
			"treeURL":       treeURL,
			"getURL":        getURL,
			"readableBytes": func(s int64) string { return humanize.Bytes(uint64(s)) },
		},
	}
}

// checkPermission checks if the user has permission to read the blob.
func checkPermission(c *router.Context, next router.Handler) {
	switch ok, err := auth.HasPermission(c.Request.Context(), permMintToken, readOnlyRealm(c.Params), nil); {
	case err != nil:
		renderErrorPage(c.Request.Context(), c.Writer, err)
	case !ok:
		err = errors.New("permission denied", grpcutil.PermissionDeniedTag)
		renderErrorPage(c.Request.Context(), c.Writer, err)
	default:
		next(c)
	}
}

// rootHandler renders top page.
func rootHandler(c *router.Context) {
	templates.MustRender(c.Request.Context(), c.Writer, "pages/index.html", nil)
}

func treeHandler(c *router.Context) {
	inst := fullInstName(c.Params)
	cl, err := GetClient(c.Request.Context(), inst)
	if err != nil {
		renderErrorPage(c.Request.Context(), c.Writer, err)
		return
	}
	bd, err := blobDigest(c.Params)
	if err != nil {
		renderErrorPage(c.Request.Context(), c.Writer, err)
		return
	}
	err = renderTree(c.Request.Context(), c.Writer, cl, bd, inst)
	if err != nil {
		renderErrorPage(c.Request.Context(), c.Writer, err)
	}
}

func getHandler(c *router.Context) {
	cl, err := GetClient(c.Request.Context(), fullInstName(c.Params))
	if err != nil {
		renderErrorPage(c.Request.Context(), c.Writer, err)
		return
	}
	bd, err := blobDigest(c.Params)
	if err != nil {
		renderErrorPage(c.Request.Context(), c.Writer, err)
		return
	}
	err = returnBlob(c.Request.Context(), c.Writer, cl, bd, fileName(c.Request))
	if err != nil {
		renderErrorPage(c.Request.Context(), c.Writer, err)
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

// fileName extracts file name from query params.
func fileName(r *http.Request) string {
	names := r.URL.Query()["filename"]
	if len(names) >= 1 {
		return names[0]
	} else {
		return ""
	}
}
