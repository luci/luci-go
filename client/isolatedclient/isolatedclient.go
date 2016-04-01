// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"bytes"
	"errors"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/luci/luci-go/client/internal/lhttp"
	"github.com/luci/luci-go/client/internal/retry"
	"github.com/luci/luci-go/client/internal/tracer"
	"github.com/luci/luci-go/common/api/isolate/isolateservice/v1"
	"github.com/luci/luci-go/common/isolated"
)

// compressedBufSize is the size of the read buffer that will be used to pull
// data from a source into the compressor.
const compressedBufSize = 4096

// Source is a generator method to return source data. A generated Source must
// be Closed before the generator is called again.
type Source func() (io.ReadCloser, error)

// NewBytesSource returns a Source implementation that reads from the supplied
// byte slice.
func NewBytesSource(d []byte) Source {
	return func() (io.ReadCloser, error) {
		return ioutil.NopCloser(bytes.NewReader(d)), nil
	}
}

// IsolateServer is the low-level client interface to interact with an Isolate
// server.
type IsolateServer interface {
	ServerCapabilities() (*isolateservice.HandlersEndpointsV1ServerDetails, error)
	// Contains looks up cache presence on the server of multiple items.
	//
	// The returned list is in the same order as 'items', with entries nil for
	// items that were present.
	Contains(items []*isolateservice.HandlersEndpointsV1Digest) ([]*PushState, error)
	Push(state *PushState, src Source) error
}

// PushState is per-item state passed from IsolateServer.Contains() to
// IsolateServer.Push().
//
// Its content is implementation specific.
type PushState struct {
	status    isolateservice.HandlersEndpointsV1PreuploadStatus
	digest    isolated.HexDigest
	size      int64
	uploaded  bool
	finalized bool
}

// New returns a new IsolateServer client.
//
// 'client' must implement authentication sufficient to talk to Isolate server
// (OAuth tokens with 'email' scope).
func New(client *http.Client, host, namespace string) IsolateServer {
	return newIsolateServer(client, host, namespace, retry.Default)
}

// Private details.

type isolateServer struct {
	config    *retry.Config
	url       string
	namespace string

	authClient *http.Client // client that sends auth tokens
	anonClient *http.Client // client that does NOT send auth tokens
}

func newIsolateServer(client *http.Client, host, namespace string, config *retry.Config) *isolateServer {
	if client == nil {
		client = http.DefaultClient
	}
	i := &isolateServer{
		config:     config,
		url:        strings.TrimRight(host, "/"),
		namespace:  namespace,
		authClient: client,
		anonClient: http.DefaultClient,
	}
	tracer.NewPID(i, "isolatedclient:"+i.url)
	return i
}

// postJSON does authenticated POST request.
func (i *isolateServer) postJSON(resource string, headers map[string]string, in, out interface{}) error {
	if len(resource) == 0 || resource[0] != '/' {
		return errors.New("resource must start with '/'")
	}
	_, err := lhttp.PostJSON(i.config, i.authClient, i.url+resource, headers, in, out)
	return err
}

func (i *isolateServer) ServerCapabilities() (*isolateservice.HandlersEndpointsV1ServerDetails, error) {
	out := &isolateservice.HandlersEndpointsV1ServerDetails{}
	if err := i.postJSON("/_ah/api/isolateservice/v1/server_details", nil, map[string]string{}, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (i *isolateServer) Contains(items []*isolateservice.HandlersEndpointsV1Digest) (out []*PushState, err error) {
	end := tracer.Span(i, "contains", tracer.Args{"number": len(items)})
	defer func() { end(tracer.Args{"err": err}) }()
	in := isolateservice.HandlersEndpointsV1DigestCollection{Items: items, Namespace: &isolateservice.HandlersEndpointsV1Namespace{}}
	in.Namespace.Namespace = i.namespace
	data := &isolateservice.HandlersEndpointsV1UrlCollection{}
	if err = i.postJSON("/_ah/api/isolateservice/v1/preupload", nil, in, data); err != nil {
		return nil, err
	}
	out = make([]*PushState, len(items))
	for _, e := range data.Items {
		index := int(e.Index)
		out[index] = &PushState{
			status: *e,
			digest: isolated.HexDigest(items[index].Digest),
			size:   items[index].Size,
		}
	}
	return out, nil
}

func (i *isolateServer) Push(state *PushState, source Source) (err error) {
	// This push operation may be a retry after failed finalization call below,
	// no need to reupload contents in that case.
	if !state.uploaded {
		// PUT file to uploadURL.
		if err = i.doPush(state, source); err != nil {
			log.Printf("doPush(%s) failed: %s\n%#v", state.digest, err, state)
			return
		}
		state.uploaded = true
	}

	// Optionally notify the server that it's done.
	if state.status.GsUploadUrl != "" {
		end := tracer.Span(i, "finalize", nil)
		defer func() { end(tracer.Args{"err": err}) }()
		// TODO(vadimsh): Calculate MD5 or CRC32C sum while uploading a file and
		// send it to isolated server. That way isolate server can verify that
		// the data safely reached Google Storage (GS provides MD5 and CRC32C of
		// stored files).
		in := isolateservice.HandlersEndpointsV1FinalizeRequest{UploadTicket: state.status.UploadTicket}
		headers := map[string]string{"Cache-Control": "public, max-age=31536000"}
		if err = i.postJSON("/_ah/api/isolateservice/v1/finalize_gs_upload", headers, in, nil); err != nil {
			log.Printf("Push(%s) (finalize) failed: %s\n%#v", state.digest, err, state)
			return
		}
	}
	state.finalized = true
	return
}

func (i *isolateServer) doPush(state *PushState, source Source) (err error) {
	useDB := state.status.GsUploadUrl == ""
	end := tracer.Span(i, "push", tracer.Args{"useDB": useDB, "size": state.size})
	defer func() { end(tracer.Args{"err": err}) }()
	if useDB {
		src, err := source()
		if err != nil {
			return err
		}
		defer src.Close()

		err = i.doPushDB(state, src)
	} else {
		err = i.doPushGCS(state, source)
	}
	if err != nil {
		tracer.CounterAdd(i, "bytesUploaded", float64(state.size))
	}
	return err
}

func (i *isolateServer) doPushDB(state *PushState, reader io.Reader) error {
	buf := bytes.Buffer{}
	compressor := isolated.GetCompressor(&buf)
	if _, err := io.Copy(compressor, reader); err != nil {
		return err
	}
	if err := compressor.Close(); err != nil {
		return err
	}
	in := &isolateservice.HandlersEndpointsV1StorageRequest{UploadTicket: state.status.UploadTicket, Content: buf.Bytes()}
	return i.postJSON("/_ah/api/isolateservice/v1/store_inline", nil, in, nil)
}

func (i *isolateServer) doPushGCS(state *PushState, source Source) (err error) {
	// GsUploadUrl is signed Google Storage URL that doesn't require additional
	// authentication. In fact, using authClient causes HTTP 403 because
	// authClient's tokens don't have Cloud Storage OAuth scope. Use anonymous
	// client instead.
	req := lhttp.NewRequest(i.anonClient, func() (*http.Request, error) {
		src, err := source()
		if err != nil {
			return nil, err
		}

		request, err := http.NewRequest("PUT", state.status.GsUploadUrl, nil)
		if err != nil {
			src.Close()
			return nil, err
		}
		request.Body = newCompressed(src)
		request.Header.Set("Content-Type", "application/octet-stream")
		return request, nil
	}, func(resp *http.Response) error {
		_, err4 := io.Copy(ioutil.Discard, resp.Body)
		err5 := resp.Body.Close()
		if err4 != nil {
			return err4
		}
		return err5
	})
	return i.config.Do(req)
}

// compressed is an io.ReadCloser that transparently compresses source data in
// a separate goroutine.
type compressed struct {
	io.ReadCloser
}

func newCompressed(src io.Reader) *compressed {
	pr, pw := io.Pipe()
	go func() {
		// The compressor itself is not thread safe.
		compressor := isolated.GetCompressor(pw)

		buf := make([]byte, compressedBufSize)
		pw.CloseWithError(func() error {
			if _, err := io.CopyBuffer(compressor, src, buf); err != nil {
				return err
			}
			return compressor.Close()
		}())
	}()

	return &compressed{pr}
}
