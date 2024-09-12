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

package botapi

import (
	"net/http"

	"go.chromium.org/luci/server/router"
)

// BotCode serves the bot archive blob or an HTTP redirect to it.
//
// Used to bootstrap bots and by the self-updating bots.
//
// Uses optional "Version" route parameter. Its value is either a concrete bot
// archive digest to fetch, or an empty string (in which case the handler will
// serve a redirect to the current stable bot version).
//
// Attempts are made to utilize GAE's edge cache by setting the corresponding
// headers.
func (srv *BotAPIServer) BotCode(ctx *router.Context) {
	// TODO: Implement.
	ctx.Writer.WriteHeader(http.StatusServiceUnavailable)
}
