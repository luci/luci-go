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
	"os"
	"strings"

	"cloud.google.com/go/compute/metadata"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/iap"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers install CAS Viewer handlers to the router.
func InstallHandlers(r *router.Router, cc *ClientCache, authMW router.Middleware) {
	if authMW == nil {
		authMW = iapAuthMW()
	}
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

func iapAuthMW() router.Middleware {
	aud, err := authAudience()
	if err != nil {
		// We can do nothing if we can't get audience.
		panic(err)
	}
	authMethods := []auth.Method{
		&iap.IAPAuthMethod{Aud: aud},
	}
	a := &auth.Authenticator{Methods: authMethods}
	return a.GetMiddleware()
}

func authAudience() (string, error) {
	mcl := metadata.NewClient(nil)
	pID, err := mcl.NumericProjectID()
	if err != nil {
		return "", err
	}
	appID := strings.Split(os.Getenv("GAE_APPLICATION"), "~")[1]
	return iap.AudForGAE(pID, appID), nil
}

func rootHanlder(c *router.Context) {
	// TODO(crbug.com/1121471): Add top page.
	logging.Debugf(c.Context, "Hello world")
	hello := fmt.Sprintf(
		"Hello, world. This is CAS Viewer. Identity: %v", auth.CurrentUser(c.Context).Identity)
	c.Writer.Write([]byte(hello))
}
