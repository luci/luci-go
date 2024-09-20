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

package auth

import (
	"context"
	"net/http"
)

// NewModifyingTransport returns a transport that can modify headers of
// http.Request's that pass through it (e.g. by appending authentication
// information).
//
// Go docs explicitly prohibit this kind of behavior, but everyone cheat and
// implement it anyway. E.g. https://github.com/golang/oauth2.
//
// Works only with Go >=1.5 transports (that use Request.Cancel instead of
// deprecated Transport.CancelRequest). For same reason doesn't implement
// CancelRequest itself.
//
// TODO(vadimsh): Move it elsewhere. It has no direct relation to auth.
func NewModifyingTransport(base http.RoundTripper, modifier func(*http.Request) error) http.RoundTripper {
	return &modifyingTransport{
		base:     base,
		modifier: modifier,
	}
}

type modifyingTransport struct {
	base     http.RoundTripper
	modifier func(*http.Request) error
}

func (t *modifyingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := *req
	clone.Header = make(http.Header, len(req.Header))
	for k, v := range req.Header {
		clone.Header[k] = v
	}
	if err := t.modifier(&clone); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(&clone)
}

func (t *modifyingTransport) CloseIdleConnections() {
	type closeIdler interface {
		CloseIdleConnections()
	}
	if tr, ok := t.base.(closeIdler); ok {
		tr.CloseIdleConnections()
	}
}

var globalInstrumentTransport func(context.Context, http.RoundTripper, string) http.RoundTripper

// SetMonitoringInstrumentation sets a global callback used by Authenticator to
// add monitoring instrumentation to a transport.
//
// This is used by tsmon library in init(). We have to resort to callbacks to
// break module dependency cycle (tsmon is using auth lib already).
func SetMonitoringInstrumentation(cb func(context.Context, http.RoundTripper, string) http.RoundTripper) {
	globalInstrumentTransport = cb
}
