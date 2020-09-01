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
	"io/ioutil"
	"net/http"
	"os"

	"github.com/julienschmidt/httprouter"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/iap"
	"go.chromium.org/luci/server/router"
)

const metadataEndpointRoot = "http://metadata.google.internal/computeMetadata/v1"

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache, v iap.IDTokenValidator) {
	baseMW := router.NewMiddlewareChain(
		authMW(cc.ctx, v),
	)
	blobMW := baseMW.Extend(
		withClientCacheMW(cc),
	)

	r.GET("/", baseMW, rootHanlder)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size/tree", blobMW, treeHandler)
	r.GET("/projects/:project/instances/:instance/blobs/:hash/:size/", blobMW, getHandler)
}

func authMW(c context.Context, v iap.IDTokenValidator) router.Middleware {
	aud := iap.AudForGAE(numericProjectID(), os.Getenv("GOOGLE_CLOUD_PROJECT"))
	logging.Debugf(c, "%v\n", aud)
	authMethods := []auth.Method{
		&iap.IAPAuthMethod{Aud: aud, Validator: v},
	}
	a := &auth.Authenticator{Methods: authMethods}
	return a.GetMiddleware()
}

// numericProjectID returns the numeric project ID by accessing the metadata server.
func numericProjectID() string {
	resp, err := http.Get(metadataEndpointRoot + "/project/numeric-project-id")
	if err != nil {
		panic(err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}
	return string(body)
}

func rootHanlder(c *router.Context) {
	// TODO(crbug.com/1121471): Add top page.
	logging.Debugf(c.Context, "Hello world")
	hello := fmt.Sprintf(
		"Hello, world. This is CAS Viewer. Identity: %v", auth.CurrentUser(c.Context).Identity)
	c.Writer.Write([]byte(hello))
}

func treeHandler(c *router.Context) {
	_, err := GetClient(c.Context, fullInstanceName(c.Params))
	if err != nil {
		errMsg := "failed to initialize CAS client"
		logging.Errorf(c.Context, "%s: %s", errMsg, err)
		http.Error(c.Writer, errMsg, http.StatusInternalServerError)
		return
	}

	// TODO(crbug.com/1121471): retrieve blob and render html.
}

func getHandler(c *router.Context) {
	_, err := GetClient(c.Context, fullInstanceName(c.Params))
	if err != nil {
		errMsg := "failed to initialize CAS client"
		logging.Errorf(c.Context, "%s: %s", errMsg, err)
		http.Error(c.Writer, errMsg, http.StatusInternalServerError)
		return
	}

	// TODO(crbug.com/1121471): retrieve blob.
}

func fullInstanceName(p httprouter.Params) string {
	return fmt.Sprintf(
		"projects/%s/instances/%s", p.ByName(":project"), p.ByName(":instance"))
}

func fullResourceName(p httprouter.Params) string {
	return fmt.Sprintf(
		"%s/blobs/%s/%s", fullInstanceName(p), p.ByName(":hash"), p.ByName(":size"))
}
