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

package main

import (
	"context"

	"cloud.google.com/go/compute/metadata"

	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/iap"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/casviewer"
)

func main() {
	server.Main(nil, nil, func(srv *server.Server) error {
		cc := casviewer.NewClientCache(srv.Context)
		srv.RegisterCleanup(func(context.Context) { cc.Clear() })

		authMW, err := iapAuthMW()
		if err != nil {
			return err
		}
		srv.Routes.Use(router.NewMiddlewareChain(
			authMW,
		))
		casviewer.InstallHandlers(srv.Routes, cc, srv.Options.ImageVersion())
		return nil
	})
}

// iapAuthMW returns authentication middleware with IAPAuthMethod.
func iapAuthMW() (router.Middleware, error) {
	aud, err := authAudience()
	if err != nil {
		return nil, err
	}
	return auth.Authenticate(
		&iap.IAPAuthMethod{Aud: aud}), nil
}

func authAudience() (string, error) {
	c := metadata.NewClient(nil)
	pID, err := c.NumericProjectID()
	if err != nil {
		return "", err
	}
	// Cloud Project ID is AppEngine Application ID.
	appID, err := c.ProjectID()
	if err != nil {
		return "", err
	}
	return iap.AudForGAE(pID, appID), nil
}
