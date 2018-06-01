// Copyright 2018 The LUCI Authors.
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

package standard

import (
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/urlfetch"
)

var background = context.Background()

// contextAwareUrlFetch implements http.RoundTripper by instantiating GAE's
// urlfetch.Transport for each request.
//
// Requests done through GAE's urlfetch.Transport inherit the deadline of
// a context used to create the transport, totally ignoring the deadline in the
// request's context. This leads to surprising bugs.
//
// contextAwareUrlFetch works around this problem by instantiating a new
// urlfetch.Transport for each request, using request's context as a basis
// (if available), and falling back to the context provided during the creation
// otherwise.
type contextAwareUrlFetch struct {
	c context.Context
}

// RoundTrip is part of http.RoundTripper interface.
func (c *contextAwareUrlFetch) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	// We assume context.Background() always returns exact same object.
	if ctx == background {
		ctx = c.c
	}
	return urlfetch.Get(ctx).RoundTrip(r)
}
