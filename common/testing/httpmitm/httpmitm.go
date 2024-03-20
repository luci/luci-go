// Copyright 2015 The LUCI Authors.
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

// Package httpmitm contains an HTTP client that logs all incoming and outgoing
// requests.
package httpmitm

import (
	"bytes"
	"io"
	"net/http"
)

// Origin is an enumeration used to annotate which type of data is being
// fed to the callback.
type Origin uint

// Log transport types.
const (
	Request Origin = iota
	Response
)

// String converts a Origin to a user-friendly string.
func (t Origin) String() string {
	switch t {
	case Request:
		return "Request"
	case Response:
		return "Response"
	default:
		return "Unknown"
	}
}

// Callback is a callback method that is invoked during HTTP communications to
// forward captured data.
type Callback func(Origin, []byte, error)

// Transport is an implementation of http.RoundTripper that logs outgoing
// requests and incoming responses.
type Transport struct {
	// Underlying RoundTripper; uses http.DefaultTransport if nil.
	http.RoundTripper

	Callback Callback // Output callback.
}

// RoundTrip implements the http.RoundTripper interface.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var buf, bodyBuf bytes.Buffer

	reqCopy := *req // Shallow copy of req, since we modify it.

	// Since "Body" is an io.Reader, it can only be read once. However, we need
	// to read it twice, once for the request capture and once for the actual
	// request.
	//
	// To that end, we will tee Body into a Buffer when we perform the initial
	// read, then replace "reqCopy.Body" with that Buffer for RoundTrip to read
	// from.
	origBody := reqCopy.Body
	if origBody != nil {
		reqCopy.Body = io.NopCloser(io.TeeReader(origBody, &bodyBuf))
	}
	reqCopy.Write(&buf)
	if origBody != nil {
		if err := origBody.Close(); err != nil {
			t.callback(Request, nil, err)
		}
		reqCopy.Body = io.NopCloser(&bodyBuf)
	}
	t.callback(Request, buf.Bytes(), nil)

	rt := t.RoundTripper
	if rt == nil {
		rt = http.DefaultTransport
	}
	res, err := rt.RoundTrip(&reqCopy)

	if err != nil {
		t.callback(Response, nil, err)
		return res, err
	}

	body := res.Body
	if body != nil {
		bodyBuf.Reset()
		res.Body = io.NopCloser(io.TeeReader(body, &bodyBuf))
		defer body.Close()
	}

	buf.Reset()
	res.Write(&buf)
	t.callback(Response, buf.Bytes(), nil)
	if body != nil {
		res.Body = io.NopCloser(&bodyBuf)
	}
	return res, nil
}

func (t *Transport) callback(o Origin, data []byte, err error) {
	if t.Callback != nil {
		t.Callback(o, data, err)
	}
}
