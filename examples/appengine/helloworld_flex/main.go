// Copyright 2017 The LUCI Authors.
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
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/appengine/gaemiddleware/flex"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/server/router"
)

func main() {
	mathrand.SeedRandomly()

	// Initialize the root context.
	c := gologger.StdConfig.Use(context.Background())

	// Prepare the router.
	r := router.NewWithRootContext(c)
	flex.ReadOnlyFlex.InstallHandlers(r)
	r.GET("/", flex.ReadOnlyFlex.Base(), indexPage)

	// Start serving.
	logging.Infof(c, "Listening on %s...", flex.ListeningAddr)
	if err := http.ListenAndServe(flex.ListeningAddr, r); err != nil {
		logging.WithError(err).Errorf(c, "Failed HTTP listen.")
		panic(err)
	}
}

func indexPage(c *router.Context) {
	c.Writer.Write([]byte("Hello, world!"))
}
