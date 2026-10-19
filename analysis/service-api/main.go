// Copyright 2024 The LUCI Authors.
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
	"go.chromium.org/luci/common/errors"
	luciserver "go.chromium.org/luci/server"

	"go.chromium.org/luci/analysis/server"

	_ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
)

// main implements the entrypoint for the api service, for GAE instances.
// This is the only service accessible from analysis.api.luci.app.
func main() {
	server.Main(func(srv *luciserver.Server) error {
		// Note: Most RPCs are now served via Cloud Run.
		// Only Buganizer-related RPCs are handled via this server, even if
		// more RPCs are registered here.
		if err := server.RegisterRPCHandlers(srv); err != nil {
			return errors.Fmt("register rpc handlers: %w", err)
		}
		return nil
	})
}
