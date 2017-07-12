// Copyright 2016 The LUCI Authors.
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

package distributor

import (
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/router"
)

// InstallHandlers installs the taskqueue callback handler.
//
// The `base` middleware must have a registry installed with WithRegistry.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.POST(handlerPattern, base.Extend(gaemiddleware.RequireTaskQueue("")), TaskQueueHandler)
	r.POST("/_ah/push-handlers/"+notifyTopicSuffix, base, PubsubReceiver)
}
