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

package ui

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"

	"go.chromium.org/luci/server"
	bootstrap "go.chromium.org/luci/web/third_party/bootstrap/v5"
)

//go:embed static
var staticFS embed.FS

// InstallHandlers adds HTTP handlers that render HTML pages.
func InstallHandlers(srv *server.Server) {
	switch sFS, err := fs.Sub(staticFS, "static"); {
	case err != nil:
		panic(fmt.Errorf("failed to return a subFS for static directory: %w", err))
	default:
		// staticFS has "static" directory at top level. We need to make the child
		// directories inside the "static" directory top level directory.
		srv.Routes.Static("/static", nil, http.FS(sFS))
	}
	srv.Routes.Static("/third_party/bootstrap", nil, http.FS(bootstrap.FS))
}
