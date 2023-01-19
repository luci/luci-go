// Copyright 2023 The LUCI Authors.
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

// Binary verification implements HTTP server that handles verification tasks.
package main

import (
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/cipd/appengine/impl"

	// Using transactional datastore TQ tasks.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

func main() {
	// impl.Main registers all task queue handlers (in particular verification
	// handlers) inside.
	impl.Main(nil, func(srv *server.Server, svc *impl.Services) error {
		// Needed when using manual scaling.
		srv.Routes.GET("/_ah/start", nil, func(ctx *router.Context) {
			_, _ = ctx.Writer.Write([]byte("OK"))
		})
		srv.Routes.GET("/_ah/stop", nil, func(ctx *router.Context) {
			_, _ = ctx.Writer.Write([]byte("OK"))
		})
		return nil
	})
}
