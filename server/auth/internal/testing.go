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

package internal

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"
	"sync"
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
