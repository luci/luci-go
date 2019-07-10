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

// Binary stub doesn't actually do anything. It exists to give GAE a deployable
// target so that the static pages for this service can be available.
//
// We include a warmup handler because it is a prerequisite for application-wide
// version switching.
package main

import (
	"net/http"

	"google.golang.org/appengine"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/warmup"
)

func main() {
	r := router.New()

	base := router.NewMiddlewareChain()
	warmup.InstallHandlers(r, base)

	http.Handle("/", r)
	appengine.Main()
}
