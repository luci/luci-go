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

package taskqueue

import (
	"net/http"

	"google.golang.org/appengine/taskqueue"
)

// TODO(vadimsh): Use real type aliases once GAE is on 1.9?

// RequestHeaders are the special HTTP request headers available to push task
// HTTP request handlers.
type RequestHeaders taskqueue.RequestHeaders

// ParseRequestHeaders parses the special HTTP request headers available to push
// task request handlers. This function silently ignores values of the wrong
// format.
func ParseRequestHeaders(h http.Header) *RequestHeaders {
	return (*RequestHeaders)(taskqueue.ParseRequestHeaders(h))
}
