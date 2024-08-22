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

package app

import (
	"net/http"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/router"
)

// Sent by pubsub.
// This struct is just convenient for unwrapping the json message.
// See https://source.chromium.org/chromium/infra/infra/+/main:luci/appengine/components/components/pubsub.py;l=178;drc=78ce3aa55a2e5f77dc05517ef3ec377b3f36dc6e.
type pubsubMessage struct {
	Message struct {
		Data       []byte
		Attributes map[string]any
	}
}

func errStatus(err error) string {
	if err == nil {
		return "success"
	}
	if transient.Tag.In(err) {
		return "transient-failure"
	} else if pubsub.Ignore.In(err) {
		return "ignored"
	} else {
		return "permanent-failure"
	}
}

func processErr(ctx *router.Context, err error) string {
	if transient.Tag.In(err) {
		// Transient errors are 500 so that PubSub retries them.
		ctx.Writer.WriteHeader(http.StatusInternalServerError)
		return "transient-failure"
	} else if pubsub.Ignore.In(err) {
		// Use subtly different "success" response codes to surface in
		// standard GAE logs whether an ingestion was ignored or not,
		// while still acknowledging the pub/sub.
		// See https://cloud.google.com/pubsub/docs/push#receiving_messages.
		ctx.Writer.WriteHeader(http.StatusNoContent)
		return "ignored"
	} else {
		// Permanent failures are 202s so that:
		// - PubSub does not retry them, and
		// - the results can be distinguished from success / ignored results
		//   (which are reported as 200 OK / 204 No Content) in logs.
		// See https://cloud.google.com/pubsub/docs/push#receiving_messages.
		ctx.Writer.WriteHeader(http.StatusAccepted)
		return "permanent-failure"
	}
}
