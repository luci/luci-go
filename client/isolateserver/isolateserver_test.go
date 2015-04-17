// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolateserver

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/maruel/ut"
)

type jsonAPI func(body io.Reader) interface{}

// handlerJSON converts a jsonAPI http handler to a proper http.Handler.
func handlerJSON(t *testing.T, handler jsonAPI) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := "application/json; charset=utf-8"
		if r.Header.Get("Content-Type") != contentType {
			t.Fatalf("invalid content type: %s", r.Header.Get("Content-Type"))
		}
		defer r.Body.Close()
		out := handler(r.Body)
		w.Header().Set("Content-Type", contentType)
		j := json.NewEncoder(w)
		ut.AssertEqual(t, nil, j.Encode(out))
	})
}

func handleJSON(t *testing.T, mux *http.ServeMux, path string, handler jsonAPI) {
	mux.Handle(path, handlerJSON(t, handler))
}

type isolateServerFake struct {
	lock     sync.Mutex
	contents map[HexDigest][]byte
}

func newIsolateServerFake(t *testing.T) (http.Handler, *isolateServerFake) {
	mux := http.NewServeMux()
	server := &isolateServerFake{
		contents: map[HexDigest][]byte{},
	}

	handleJSON(t, mux, "/_ah/api/isolateservice/v1/server_details", func(body io.Reader) interface{} {
		content, err := ioutil.ReadAll(body)
		ut.AssertEqual(t, nil, err)
		ut.AssertEqual(t, []byte("{}"), content)
		return &ServerCapabilities{"v1"}
	})

	// Fail on anything else.
	mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		t.Fatal()
	})
	return mux, server
}

func TestIsolateServerCaps(t *testing.T) {
	mux, _ := newIsolateServerFake(t)
	ts := httptest.NewServer(mux)
	defer ts.Close()
	client := New(ts.URL, "default", "sha-1", "flate")
	caps, err := client.ServerCapabilities()
	ut.AssertEqual(t, nil, err)
	ut.AssertEqual(t, &ServerCapabilities{"v1"}, caps)
}
