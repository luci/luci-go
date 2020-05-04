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

	"go.chromium.org/luci/common/flag/stringmapflag"
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

	contentHostnameMap := stringmapflag.Value{}
	flag.Var(
		&contentHostnameMap,
		"user-content-host-map",
		"Key=value map where key is a ResultDB API hostname and value is a "+
			"hostname to use for user-content URLs produced there. "+
			"Key '*' indicates a fallback.")

	// TODO(vadimsh): Remove this once -user-content-host-map is rolled out.
	contentHostname := flag.String(
		"user-content-host",
		"results.usercontent.cr.dev",
		"Use this host for all user-content URLs",
	)

	internal.Main(func(srv *server.Server) error {
		if contentHostnameMap["*"] == "" && *contentHostname != "" {
			contentHostnameMap["*"] = *contentHostname
		}
		return resultdb.InitServer(srv, resultdb.Options{
			InsecureSelfURLs:   *insecureSelfURLs,
			ContentHostnameMap: map[string]string(contentHostnameMap),
		})
	})
}
