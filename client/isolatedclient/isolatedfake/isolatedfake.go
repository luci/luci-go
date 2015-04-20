// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedfake

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/luci/luci-go/common/isolated"
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

type IsolatedFake interface {
	http.Handler
	Contents() map[isolated.HexDigest][]byte
}

type isolatedFake struct {
	t        *testing.T
	mux      *http.ServeMux
	lock     sync.Mutex
	contents map[isolated.HexDigest][]byte
}

// New starts a fake in-process isolated server.
//
// Call Close() to stop the server.
func New(t *testing.T) IsolatedFake {
	server := &isolatedFake{
		t:        t,
		mux:      http.NewServeMux(),
		contents: map[isolated.HexDigest][]byte{},
	}

	server.handleJSON("/_ah/api/isolateservice/v1/server_details", server.serverDetails)
	server.handleJSON("/_ah/api/isolateservice/v1/preupload", server.preupload)
	server.handleJSON("/_ah/api/isolateservice/v1/finalize_gs_upload", server.finalizeGSUpload)
	server.handleJSON("/_ah/api/isolateservice/v1/store_inline", server.storeInline)

	// Fail on anything else.
	server.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		t.Fatal()
	})
	return server
}

// Private details.

func (server *isolatedFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server.mux.ServeHTTP(w, r)
}

func (server *isolatedFake) Contents() map[isolated.HexDigest][]byte {
	server.lock.Lock()
	defer server.lock.Unlock()
	out := map[isolated.HexDigest][]byte{}
	for k, v := range server.contents {
		out[k] = v
	}
	return out
}

func (server *isolatedFake) handleJSON(path string, handler jsonAPI) {
	server.mux.Handle(path, handlerJSON(server.t, handler))
}

func (server *isolatedFake) serverDetails(body io.Reader) interface{} {
	content, err := ioutil.ReadAll(body)
	ut.AssertEqual(server.t, nil, err)
	ut.AssertEqual(server.t, []byte("{}"), content)
	return map[string]string{"server_version": "v1"}
}

func (server *isolatedFake) preupload(body io.Reader) interface{} {
	data := &isolated.DigestCollection{}
	ut.AssertEqual(server.t, nil, json.NewDecoder(body).Decode(data))
	ut.AssertEqual(server.t, "default", data.Namespace.Namespace)
	out := &isolated.UrlCollection{}

	server.lock.Lock()
	defer server.lock.Unlock()
	for i, d := range data.Items {
		if _, ok := server.contents[d.Digest]; !ok {
			ticket := "ticket:" + string(d.Digest)
			out.Items = append(out.Items, isolated.PreuploadStatus{"", ticket, isolated.Int(i)})
		}
	}
	return out
}

func (server *isolatedFake) finalizeGSUpload(body io.Reader) interface{} {
	data := &isolated.FinalizeRequest{}
	ut.AssertEqual(server.t, nil, json.NewDecoder(body).Decode(data))

	server.lock.Lock()
	defer server.lock.Unlock()
	return map[string]string{"ok": "true"}
}

func (server *isolatedFake) storeInline(body io.Reader) interface{} {
	data := &isolated.StorageRequest{}
	ut.AssertEqual(server.t, nil, json.NewDecoder(body).Decode(data))
	prefix := "ticket:"
	ut.AssertEqual(server.t, true, strings.HasPrefix(data.UploadTicket, prefix))
	digest := isolated.HexDigest(data.UploadTicket[len(prefix):])
	ut.AssertEqual(server.t, true, digest.Validate())
	comp := isolated.GetDecompressor(bytes.NewBuffer(data.Content))
	raw, err := ioutil.ReadAll(comp)
	ut.AssertEqual(server.t, nil, err)
	ut.AssertEqual(server.t, digest, isolated.HashBytes(raw))

	server.lock.Lock()
	defer server.lock.Unlock()
	server.contents[digest] = raw
	return map[string]string{"ok": "true"}
}
