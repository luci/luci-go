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

package flex

import (
	"net/http"

	"golang.org/x/net/context"
)

var httpRequestKey = "flex/Inbound HTTP Request"

func withHTTPRequest(c context.Context, req *http.Request) context.Context {
	return context.WithValue(c, &httpRequestKey, req)
}

// HTTPRequest returns the inbound HTTP request associated with the current
// Context. This value will always be available.
func HTTPRequest(c context.Context) *http.Request {
	return c.Value(&httpRequestKey).(*http.Request)
}
