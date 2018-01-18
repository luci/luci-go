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

// Package backend implements HTTP server that handles task queues and crons.
package backend

import (
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware/standard"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/cipd/appengine/impl/common"

	// Install TQ task handlers.
	_ "go.chromium.org/luci/cipd/appengine/impl/cas"
	_ "go.chromium.org/luci/cipd/appengine/impl/repo"
)

func init() {
	r := router.New()
	standard.InstallHandlers(r)
	common.TQ.InstallRoutes(r, standard.Base())
	http.DefaultServeMux.Handle("/", r)
}
