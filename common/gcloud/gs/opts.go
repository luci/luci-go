// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gs

import (
	"fmt"
	"net/http"
)

// Options are the set of extra options to apply to the Google Storage request.
type Options struct {
	// From is the range request starting index. If >0, the beginning of the
	// range request will be set.
	From int64
	// To is the range request ending index. If >0, the end of the
	// range request will be set.
	//
	// If no From index is set, this will result in a request indexed from the end
	// of the object.
	To int64
}

// gsRoundTripper is an http.RoundTripper implementation that allows us to
// inject some HTTP headers. It is necessary because the stock Google Storage
// API doesn't allow access to specific header manipulation.
//
// https://cloud.google.com/storage/docs/reference-headers#range
type gsRoundTripper struct {
	http.RoundTripper
	*Options
}

func (rt *gsRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}

	// Add Range header, if configured.
	rh := ""
	switch {
	case rt.From > 0 && rt.To > 0:
		rh = fmt.Sprintf("bytes=%d-%d", rt.From, rt.To)

	case rt.From > 0:
		rh = fmt.Sprintf("bytes=%d-", rt.From)

	case rt.To > 0:
		rh = fmt.Sprintf("bytes=-%d", rt.To)
	}
	if rh != "" {
		req.Header.Add("Range", rh)
	}

	return rt.RoundTripper.RoundTrip(req)
}
