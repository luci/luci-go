// Copyright 2019 The LUCI Authors.
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
	"flag"

	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/resultdb"
)

func main() {
	insecureSelfURLs := flag.Bool(
		"insecure-self-urls",
		false,
		"Use http:// (not https://) for URLs pointing back to ResultDB",
	)
	contentHostname := flag.String(
		"user-content-host",
		// TODO(crbug.com/1042261): remove the default and make it required.
		// Without the default staging will start crashing.
		"results.usercontent.cr.dev",
		"Use this host for all user-content URLs",
	)

	internal.Main(func(srv *server.Server) error {
		return resultdb.InitServer(srv, resultdb.Options{
			InsecureSelfURLs: *insecureSelfURLs,
			ContentHostname:  *contentHostname,
		})
	})
}
