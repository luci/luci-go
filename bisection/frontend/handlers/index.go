// Copyright 2022 The LUCI Authors.
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

package handlers

import (
	"fmt"
	"net/http"

	"go.chromium.org/luci/server/router"
)

// Redirect returns a handler that redirect traffics to Milo.
func Redirect(redirectURL string) func(ctx *router.Context) {
	return func(ctx *router.Context) {
		url := fmt.Sprintf("https://%s%s", redirectURL, ctx.Request.URL.Path)
		http.Redirect(ctx.Writer, ctx.Request, url, http.StatusFound)
	}
}
