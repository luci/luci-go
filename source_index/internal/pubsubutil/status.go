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
)

const (
	// StatusSuccess is the status code for successful pubsub handler execution.
	//
	// Use "200 OK" so that PubSub retries them.
	StatusSuccess = http.StatusOK

	// StatusTransientFailure is the status code for transient pubsub handler
	// failures.
	//
	// Use "500 Internal Server Error" so that PubSub retries them.
	StatusTransientFailure = http.StatusInternalServerError
	// StatusPermanentFailure is the status code for permanent pubsub handler
	// failures.
	//
	// Use "202 Accepted" so that:
	// - PubSub does not retry them, and
	// - the results can be distinguished from success / ignored results
	//   (which are reported as "200 OK" / "204 No Content") in logs.
	// See https://cloud.google.com/pubsub/docs/push#receiving_messages.
	StatusPermanentFailure = http.StatusAccepted
)

// StatusCode returns the appropriate HTTP status code for the given pubsub
// handler error. Returns StatusSuccess is error is nil.
func StatusCode(err error) int {
	switch {
	case err == nil:
		return StatusSuccess
	case transient.Tag.In(err):
		return StatusTransientFailure
	default:
		return StatusPermanentFailure
	}
}

// StatusString returns the string representation for the pubsub handler status.
func StatusString(code int) string {
	switch code {
	case StatusSuccess:
		return "success"
	case StatusTransientFailure:
		return "transient-failure"
	case StatusPermanentFailure:
		return "permanent-failure"
	default:
		return "unknown"
	}
}
