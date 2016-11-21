// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package eventlog

import "net/http"

// A ClientOption is an optional argument to NewClient.
type ClientOption interface {
	apply(*clientSettings)
}

type clientSettings struct {
	HTTPClient *http.Client
}

func (cs *clientSettings) Populate(opts []ClientOption) {
}

type httpClientOption struct {
	*http.Client
}

func (opt httpClientOption) apply(cs *clientSettings) {
	cs.HTTPClient = opt.Client
}

// WithHTTPClient returns a ClientOption that specifies the HTTP client to use as the basis of communications.
func WithHTTPClient(client *http.Client) ClientOption {
	return httpClientOption{client}
}
