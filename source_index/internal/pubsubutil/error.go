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

package pubsubutil

import (
	"net/http"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/router"
)

func HandleErr(ctx *router.Context, err error) {
	if transient.Tag.In(err) {
		// Transient errors are 500 so that PubSub retries them.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
	} else {
		// Permanent failures are 202s so that:
		// - PubSub does not retry them, and
		// - the results can be distinguished from success / ignored results
		//   (which are reported as 200 OK / 204 No Content) in logs.
		// See https://cloud.google.com/pubsub/docs/push#receiving_messages.
		ctx.Writer.WriteHeader(http.StatusAccepted)
	}
}
