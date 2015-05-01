// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedfake

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"

	"github.com/luci/luci-go/common/isolated"
)

type jsonAPI func(body io.Reader) interface{}

type failure interface {
	Fail(err error)
}

// handlerJSON converts a jsonAPI http handler to a proper http.Handler.
func handlerJSON(f failure, handler jsonAPI) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType := "application/json; charset=utf-8"
		if r.Header.Get("Content-Type") != contentType {
			f.Fail(fmt.Errorf("invalid content type: %s", r.Header.Get("Content-Type")))
			return
		}
		defer r.Body.Close()
		out := handler(r.Body)
		w.Header().Set("Content-Type", contentType)
		j := json.NewEncoder(w)
		if err := j.Encode(out); err != nil {
			f.Fail(err)
		}
	})
}

type IsolatedFake interface {
	http.Handler
	Contents() map[isolated.HexDigest][]byte
	Error() error
}

type isolatedFake struct {
	mux      *http.ServeMux
	lock     sync.Mutex
	err      error
	contents map[isolated.HexDigest][]byte
}

// New starts a fake in-process isolated server.
//
// Call Close() to stop the server.
func New() IsolatedFake {
	server := &isolatedFake{
		mux:      http.NewServeMux(),
		contents: map[isolated.HexDigest][]byte{},
	}

	server.handleJSON("/_ah/api/isolateservice/v1/server_details", server.serverDetails)
	server.handleJSON("/_ah/api/isolateservice/v1/preupload", server.preupload)
	server.handleJSON("/_ah/api/isolateservice/v1/finalize_gs_upload", server.finalizeGSUpload)
	server.handleJSON("/_ah/api/isolateservice/v1/store_inline", server.storeInline)

	// Fail on anything else.
	server.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		server.Fail(fmt.Errorf("unknwown endpoint %s", req.URL))
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

func (server *isolatedFake) Fail(err error) {
	server.lock.Lock()
	defer server.lock.Unlock()
	if server.err == nil {
		server.err = err
	}
}

func (server *isolatedFake) Error() error {
	server.lock.Lock()
	defer server.lock.Unlock()
	return server.err
}

func (server *isolatedFake) handleJSON(path string, handler jsonAPI) {
	server.mux.Handle(path, handlerJSON(server, handler))
}

func (server *isolatedFake) serverDetails(body io.Reader) interface{} {
	content, err := ioutil.ReadAll(body)
	if err != nil {
		server.Fail(err)
	}
	if string(content) != "{}" {
		server.Fail(fmt.Errorf("unexpected content %#v", string(content)))
	}
	return map[string]string{"server_version": "v1"}
}

func (server *isolatedFake) preupload(body io.Reader) interface{} {
	data := &isolated.DigestCollection{}
	if err := json.NewDecoder(body).Decode(data); err != nil {
		server.Fail(err)
	}
	if data.Namespace.Namespace != "default-gzip" {
		server.Fail(fmt.Errorf("unexpected namespace %#v", data.Namespace.Namespace))
	}
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
	if err := json.NewDecoder(body).Decode(data); err != nil {
		server.Fail(err)
	}

	server.lock.Lock()
	defer server.lock.Unlock()
	return map[string]string{"ok": "true"}
}

func (server *isolatedFake) storeInline(body io.Reader) interface{} {
	data := &isolated.StorageRequest{}
	if err := json.NewDecoder(body).Decode(data); err != nil {
		server.Fail(err)
	}

	prefix := "ticket:"
	if !strings.HasPrefix(data.UploadTicket, prefix) {
		server.Fail(fmt.Errorf("unexpected ticket %#v", data.UploadTicket))
	}

	digest := isolated.HexDigest(data.UploadTicket[len(prefix):])
	if !digest.Validate() {
		server.Fail(fmt.Errorf("invalid digest %#v", digest))
	}
	comp := isolated.GetDecompressor(bytes.NewBuffer(data.Content))
	raw, err := ioutil.ReadAll(comp)
	if err != nil {
		server.Fail(err)
	}
	if digest != isolated.HashBytes(raw) {
		server.Fail(fmt.Errorf("invalid digest %#v", digest))
	}

	server.lock.Lock()
	defer server.lock.Unlock()
	server.contents[digest] = raw
	return map[string]string{"ok": "true"}
}
