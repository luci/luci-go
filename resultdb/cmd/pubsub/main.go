// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"flag"

	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/pubsub"
)

func main() {
	opts := pubsub.Options{}
	flag.StringVar(
		&opts.ResultDBHostname,
		"resultdb-host",
		"",
		"The hostname of the luci.resultdb.v1.ResultDB service instance, e.g results.api.cr.dev",
	)

	internal.Main(func(srv *server.Server) error {
		return pubsub.InitServer(srv)
	})
}
