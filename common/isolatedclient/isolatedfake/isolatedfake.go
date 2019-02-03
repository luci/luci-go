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

// Package isolatedfake implements an in-process fake Isolated server for
// integration testing.
package isolatedfake

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"

	isolateservice "go.chromium.org/luci/common/api/isolate/isolateservice/v1"
	"go.chromium.org/luci/common/isolated"
)

const contentType = "application/json; charset=utf-8"

type jsonAPI func(r *http.Request) interface{}

type failure interface {
	Fail(err error)
}

// handlerJSON converts a jsonAPI http handler to a proper http.Handler.
func handlerJSON(f failure, handler jsonAPI) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		//log.Printf("%s", r.URL)
		if r.Header.Get("Content-Type") != contentType {
			f.Fail(fmt.Errorf("invalid content type: %q", r.Header.Get("Content-Type")))
			return
		}
		defer r.Body.Close()
		out := handler(r)
		w.Header().Set("Content-Type", contentType)
		j := json.NewEncoder(w)
		if err := j.Encode(out); err != nil {
			f.Fail(err)
		}
	})
}

func gsURLWithDigestQuery(host, path, namespace, digest string) *url.URL {
	v := url.Values{}
	v.Add("namespace", namespace)
	v.Add("digest", digest)
	fullPath := fmt.Sprintf("/fake/cloudstorage/%s", path)
	return &url.URL{Scheme: "http", Host: host, Path: fullPath, RawQuery: v.Encode()}
}

// IsolatedFake is a functional fake in-memory isolated server.
type IsolatedFake interface {
	http.Handler
	// Contents returns all the uncompressed data on the fake isolated server,
	// per namespace.
	Contents() map[string]map[isolated.HexDigest][]byte
	// Inject adds uncompressed data in the fake isolated server.
	Inject(namespace string, data []byte) isolated.HexDigest
	Error() error
}

type isolatedFake struct {
	mux *http.ServeMux

	mu       sync.Mutex
	err      error
	contents map[string]map[isolated.HexDigest][]byte
	staging  map[string]map[isolated.HexDigest][]byte // Uploaded to GCS but not yet finalized.
}

// New create a HTTP router that implements an isolated server.
func New() IsolatedFake {
	s := &isolatedFake{
		mux:      http.NewServeMux(),
		contents: map[string]map[isolated.HexDigest][]byte{},
		staging:  map[string]map[isolated.HexDigest][]byte{},
	}

	s.handleJSON("/_ah/api/isolateservice/v1/server_details", s.serverDetails)
	s.handleJSON("/_ah/api/isolateservice/v1/preupload", s.preupload)
	s.handleJSON("/_ah/api/isolateservice/v1/finalize_gs_upload", s.finalizeGSUpload)
	s.handleJSON("/_ah/api/isolateservice/v1/store_inline", s.storeInline)
	s.handleJSON("/_ah/api/isolateservice/v1/retrieve", s.retrieve)
	s.mux.HandleFunc("/fake/cloudstorage/upload", s.fakeCloudStorageUpload)
	s.mux.HandleFunc("/fake/cloudstorage/download", s.fakeCloudStorageDownload)

	// Fail on anything else.
	s.mux.HandleFunc("/", func(w http.ResponseWriter, req *http.Request) {
		s.Fail(fmt.Errorf("unknown endpoint %q", req.URL))
	})
	return s
}

// Private details.

func (s *isolatedFake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *isolatedFake) Contents() map[string]map[isolated.HexDigest][]byte {
	// Make a copy of the maps for safety. Only the actual content is not copied.
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]map[isolated.HexDigest][]byte{}
	for namespace, src := range s.contents {
		out[namespace] = map[isolated.HexDigest][]byte{}
		for k, v := range src {
			out[namespace][k] = v
		}
	}
	return out
}

func (s *isolatedFake) Inject(namespace string, data []byte) isolated.HexDigest {
	h := isolated.HashBytes(isolated.GetHash(namespace), data)
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.contents[namespace]; !ok {
		s.contents[namespace] = map[isolated.HexDigest][]byte{}
	}
	s.contents[namespace][h] = data
	return h
}

func (s *isolatedFake) Fail(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failLocked(err)
}

func (s *isolatedFake) Error() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *isolatedFake) failLocked(err error) {
	if s.err == nil {
		s.err = err
	}
}

func (s *isolatedFake) handleJSON(path string, handler jsonAPI) {
	s.mux.Handle(path, handlerJSON(s, handler))
}

func (s *isolatedFake) serverDetails(r *http.Request) interface{} {
	content, err := ioutil.ReadAll(r.Body)
	if err != nil {
		s.Fail(err)
	}
	if string(content) != "{}" {
		s.Fail(fmt.Errorf("unexpected content %#v", string(content)))
	}
	return map[string]string{"server_version": "v1"}
}

func (s *isolatedFake) preupload(r *http.Request) interface{} {
	data := &isolateservice.HandlersEndpointsV1DigestCollection{}
	if err := json.NewDecoder(r.Body).Decode(data); err != nil {
		s.Fail(err)
	}
	if data.Namespace == nil {
		s.Fail(fmt.Errorf("unexpected namespace %#v", data.Namespace.Namespace))
	}
	out := &isolateservice.HandlersEndpointsV1UrlCollection{}
	namespace := data.Namespace.Namespace
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, d := range data.Items {
		if _, ok := s.contents[namespace]; !ok {
			s.contents[namespace] = map[isolated.HexDigest][]byte{}
		}
		if _, ok := s.contents[namespace][isolated.HexDigest(d.Digest)]; !ok {
			// Simulate a write to Cloud Storage for larger writes.
			s := &isolateservice.HandlersEndpointsV1PreuploadStatus{
				Index:        int64(i),
				UploadTicket: "ticket:" + namespace + "," + string(d.Digest),
			}
			if d.Size > 1024 {
				s.GsUploadUrl = gsURLWithDigestQuery(r.Host, "upload", namespace, d.Digest).String()
			}
			out.Items = append(out.Items, s)
		}
	}
	return out
}

func (s *isolatedFake) fakeCloudStorageUpload(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		w.WriteHeader(400)
		s.Fail(fmt.Errorf("invalid content type: %q", r.Header.Get("Content-Type")))
		return
	}
	if r.Method != "PUT" {
		w.WriteHeader(405)
		s.Fail(fmt.Errorf("invalid method: %q", r.Method))
		return
	}
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		w.WriteHeader(400)
		s.Fail(fmt.Errorf("missing namespace"))
		return
	}
	decompressor, err := isolated.GetDecompressor(namespace, r.Body)
	if err != nil {
		w.WriteHeader(500)
		s.Fail(err)
		return
	}
	defer decompressor.Close()
	raw, err := ioutil.ReadAll(decompressor)
	if err != nil {
		w.WriteHeader(500)
		s.Fail(err)
		return
	}
	digest := isolated.HexDigest(r.URL.Query().Get("digest"))
	if digest != isolated.HashBytes(isolated.GetHash(namespace), raw) {
		w.WriteHeader(400)
		s.Fail(fmt.Errorf("invalid digest %#v", digest))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.staging[namespace]; !ok {
		s.staging[namespace] = map[isolated.HexDigest][]byte{}
	}
	s.staging[namespace][digest] = raw
	w.WriteHeader(200)
}

func (s *isolatedFake) fakeCloudStorageDownload(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	if r.Method != "GET" {
		w.WriteHeader(405)
		s.Fail(fmt.Errorf("invalid method: %q", r.Method))
		return
	}
	namespace := r.URL.Query().Get("namespace")
	store, ok := s.contents[namespace]
	if !ok {
		w.WriteHeader(404)
		s.Fail(fmt.Errorf("namespace not found: %q", namespace))
		return
	}
	digest := isolated.HexDigest(r.URL.Query().Get("digest"))
	data, ok := store[digest]
	if !ok {
		w.WriteHeader(404)
		s.Fail(fmt.Errorf("file not found: %q", digest))
		return
	}
	var buf bytes.Buffer
	compressor, err := isolated.GetCompressor(namespace, &buf)
	if err != nil {
		w.WriteHeader(500)
		s.Fail(err)
		return
	}
	if _, err := io.CopyBuffer(compressor, bytes.NewReader(data), nil); err != nil {
		compressor.Close()
		w.WriteHeader(500)
		s.Fail(err)
		return
	}
	if err := compressor.Close(); err != nil {
		w.WriteHeader(500)
		s.Fail(err)
		return
	}
	w.Write(buf.Bytes())
}

func (s *isolatedFake) finalizeGSUpload(r *http.Request) interface{} {
	data := &isolateservice.HandlersEndpointsV1FinalizeRequest{}
	if err := json.NewDecoder(r.Body).Decode(data); err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	prefix := "ticket:"
	if !strings.HasPrefix(data.UploadTicket, prefix) {
		err := fmt.Errorf("unexpected ticket %#v", data.UploadTicket)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	parts := strings.SplitN(data.UploadTicket[len(prefix):], ",", 2)
	if len(parts) != 2 {
		err := fmt.Errorf("unexpected ticket %#v", data.UploadTicket)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	namespace := parts[0]
	digest := isolated.HexDigest(parts[1])
	if !digest.Validate(isolated.GetHash(namespace)) {
		err := fmt.Errorf("invalid digest %#v", digest)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.staging[namespace]; !ok {
		err := fmt.Errorf("finalizing non uploaded file in unknown namespace")
		s.failLocked(err)
		return map[string]string{"err": err.Error()}
	}
	if _, ok := s.staging[namespace][digest]; !ok {
		err := fmt.Errorf("finalizing non uploaded file")
		s.failLocked(err)
		return map[string]string{"err": err.Error()}
	}
	if _, ok := s.contents[namespace]; !ok {
		s.contents[namespace] = map[isolated.HexDigest][]byte{}
	}
	s.contents[namespace][digest] = s.staging[namespace][digest]
	delete(s.staging[namespace], digest)
	return map[string]string{"ok": "true"}
}

func (s *isolatedFake) storeInline(r *http.Request) interface{} {
	data := &isolateservice.HandlersEndpointsV1StorageRequest{}
	if err := json.NewDecoder(r.Body).Decode(data); err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}

	prefix := "ticket:"
	if !strings.HasPrefix(data.UploadTicket, prefix) {
		err := fmt.Errorf("unexpected ticket %#v", data.UploadTicket)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}

	parts := strings.SplitN(data.UploadTicket[len(prefix):], ",", 2)
	if len(parts) != 2 {
		err := fmt.Errorf("unexpected ticket %#v", data.UploadTicket)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	namespace := parts[0]
	digest := isolated.HexDigest(parts[1])
	h := isolated.GetHash(namespace)
	if !digest.Validate(h) {
		err := fmt.Errorf("invalid digest %#v", digest)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	blob, err := base64.StdEncoding.DecodeString(data.Content)
	if err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	decompressor, err := isolated.GetDecompressor(namespace, bytes.NewReader(blob))
	if err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	defer decompressor.Close()
	raw, err := ioutil.ReadAll(decompressor)
	if err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	if digest != isolated.HashBytes(h, raw) {
		err := fmt.Errorf("invalid digest %#v", digest)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.contents[namespace]; !ok {
		s.contents[namespace] = map[isolated.HexDigest][]byte{}
	}
	s.contents[namespace][digest] = raw
	return map[string]string{"ok": "true"}
}

func (s *isolatedFake) retrieve(r *http.Request) interface{} {
	data := &isolateservice.HandlersEndpointsV1RetrieveRequest{}
	if err := json.NewDecoder(r.Body).Decode(data); err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	digest := isolated.HexDigest(data.Digest)
	namespace := data.Namespace.Namespace
	if _, ok := s.contents[namespace]; !ok {
		err := fmt.Errorf("no such digest %#v", digest)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	rawContent, ok := s.contents[namespace][digest]
	if !ok {
		err := fmt.Errorf("no such digest %#v", digest)
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	if len(rawContent) > 1024 {
		return &isolateservice.HandlersEndpointsV1RetrievedContent{
			Url: gsURLWithDigestQuery(r.Host, "download", namespace, data.Digest).String(),
		}
	}

	// Since we decompress when we get the data, we need to recompress when
	// something is fetched.
	var buf bytes.Buffer
	compressor, err := isolated.GetCompressor(namespace, &buf)
	if err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	if _, err := io.CopyBuffer(compressor, bytes.NewReader(rawContent), nil); err != nil {
		compressor.Close()
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	if err := compressor.Close(); err != nil {
		s.Fail(err)
		return map[string]string{"err": err.Error()}
	}
	return &isolateservice.HandlersEndpointsV1RetrievedContent{
		Content: base64.StdEncoding.EncodeToString(buf.Bytes()),
	}
}
