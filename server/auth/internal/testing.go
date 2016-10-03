// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package internal

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sync"

	"golang.org/x/net/context"
)

var testTransportKey = "testTransport"

// TestTransportCallback is used from unit tests.
type TestTransportCallback func(r *http.Request, body string) (code int, response string)

// WithTestTransport puts a testing transport in the context to use for fetches.
func WithTestTransport(c context.Context, cb TestTransportCallback) context.Context {
	return context.WithValue(c, &testTransportKey, &testTransport{cb: cb})
}

type testTransport struct {
	lock sync.Mutex
	cb   TestTransportCallback
}

func (t *testTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.lock.Lock()
	defer t.lock.Unlock()
	body, err := ioutil.ReadAll(r.Body)
	r.Body.Close()
	if err != nil {
		return nil, err
	}
	code, resp := t.cb(r, string(body))
	return &http.Response{
		StatusCode: code,
		Body:       ioutil.NopCloser(bytes.NewReader([]byte(resp))),
	}, nil
}
