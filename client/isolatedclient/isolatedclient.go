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
	"sync"

	"github.com/luci/luci-go/client/internal/lhttp"
	"github.com/luci/luci-go/client/internal/retry"
	"github.com/luci/luci-go/client/internal/tracer"
	"github.com/luci/luci-go/common/isolated"
)

// IsolateServer is the low-level client interface to interact with an Isolate
// server.
type IsolateServer interface {
	ServerCapabilities() (*isolated.ServerCapabilities, error)
	// Contains looks up cache presence on the server of multiple items.
	//
	// The returned list is in the same order as 'items', with entries nil for
	// items that were present.
	Contains(items []*isolated.DigestItem) ([]*PushState, error)
	Push(state *PushState, src io.ReadSeeker) error
}

// PushState is per-item state passed from IsolateServer.Contains() to
// IsolateServer.Push().
//
// Its content is implementation specific.
type PushState struct {
	status    isolated.PreuploadStatus
	digest    isolated.HexDigest
	size      int64
	uploaded  bool
	finalized bool
}

// New returns a new IsolateServer client.
func New(client *http.Client, host, namespace string) IsolateServer {
	return newIsolateServer(client, host, namespace, retry.Default)
}

// Private details.

type isolateServer struct {
	config    *retry.Config
	url       string
	namespace string
	client    *http.Client
}

func newIsolateServer(client *http.Client, host, namespace string, config *retry.Config) *isolateServer {
	if client == nil {
		client = http.DefaultClient
	}
	i := &isolateServer{
		config:    config,
		url:       strings.TrimRight(host, "/"),
		namespace: namespace,
		client:    client,
	}
	tracer.NewPID(i, "isolatedclient:"+i.url)
	return i
}

func (i *isolateServer) postJSON(resource string, in, out interface{}) error {
	if len(resource) == 0 || resource[0] != '/' {
		return errors.New("resource must start with '/'")
	}
	_, err := lhttp.PostJSON(i.config, i.client, i.url+resource, in, out)
	return err
}

func (i *isolateServer) ServerCapabilities() (*isolated.ServerCapabilities, error) {
	out := &isolated.ServerCapabilities{}
	if err := i.postJSON("/_ah/api/isolateservice/v1/server_details", map[string]string{}, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (i *isolateServer) Contains(items []*isolated.DigestItem) (out []*PushState, err error) {
	end := tracer.Span(i, "contains", tracer.Args{"number": len(items)})
	defer func() { end(tracer.Args{"err": err}) }()
	in := isolated.DigestCollection{Items: items}
	in.Namespace.Namespace = i.namespace
	data := &isolated.URLCollection{}
	if err = i.postJSON("/_ah/api/isolateservice/v1/preupload", in, data); err != nil {
		return nil, err
	}
	out = make([]*PushState, len(items))
	for _, e := range data.Items {
		index := int(e.Index)
		out[index] = &PushState{
			status: e,
			digest: items[index].Digest,
			size:   items[index].Size,
		}
	}
	return out, nil
}

func (i *isolateServer) Push(state *PushState, src io.ReadSeeker) (err error) {
	// This push operation may be a retry after failed finalization call below,
	// no need to reupload contents in that case.
	if !state.uploaded {
		// PUT file to uploadURL.
		if err = i.doPush(state, src); err != nil {
			log.Printf("doPush(%s) failed: %s\n%#v", state.digest, err, state)
			return
		}
		state.uploaded = true
	}

	// Optionally notify the server that it's done.
	if state.status.GSUploadURL != "" {
		end := tracer.Span(i, "finalize", nil)
		defer func() { end(tracer.Args{"err": err}) }()
		// TODO(vadimsh): Calculate MD5 or CRC32C sum while uploading a file and
		// send it to isolated server. That way isolate server can verify that
		// the data safely reached Google Storage (GS provides MD5 and CRC32C of
		// stored files).
		in := isolated.FinalizeRequest{state.status.UploadTicket}
		if err = i.postJSON("/_ah/api/isolateservice/v1/finalize_gs_upload", in, nil); err != nil {
			log.Printf("Push(%s) (finalize) failed: %s\n%#v", state.digest, err, state)
			return
		}
	}
	state.finalized = true
	return
}

func (i *isolateServer) doPush(state *PushState, src io.ReadSeeker) (err error) {
	useDB := state.status.GSUploadURL == ""
	end := tracer.Span(i, "push", tracer.Args{"useDB": useDB, "size": state.size})
	defer func() { end(tracer.Args{"err": err}) }()
	if useDB {
		err = i.doPushDB(state, src)
	} else {
		err = i.doPushGCS(state, src)
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
	in := &isolated.StorageRequest{state.status.UploadTicket, buf.Bytes()}
	return i.postJSON("/_ah/api/isolateservice/v1/store_inline", in, nil)
}

func (i *isolateServer) doPushGCS(state *PushState, src io.ReadSeeker) (err error) {
	c := newCompressed(src)
	defer func() {
		if err1 := c.Close(); err == nil {
			err = err1
		}
	}()
	request, err2 := http.NewRequest("PUT", state.status.GSUploadURL, c)
	if err2 != nil {
		return err2
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	req, err3 := lhttp.NewRequest(i.client, request, func(resp *http.Response) error {
		_, err4 := io.Copy(ioutil.Discard, resp.Body)
		err5 := resp.Body.Close()
		if err4 != nil {
			return err4
		}
		return err5
	})
	if err3 != nil {
		return err3
	}
	return i.config.Do(req)
}

// compressed transparently compresses a source.
//
// It supports seeking to the beginning of the file to enable re-reading the
// file multiple times. This is needed for HTTP retries.
type compressed struct {
	src io.ReadSeeker
	wg  sync.WaitGroup
	r   io.ReadCloser
}

func newCompressed(src io.ReadSeeker) *compressed {
	c := &compressed{src: src}
	c.reset()
	return c
}

func (c *compressed) Close() error {
	var err error
	if c.r != nil {
		err = c.r.Close()
		c.r = nil
	}
	c.wg.Wait()
	return err
}

// Seek resets the compressor.
func (c *compressed) Seek(offset int64, whence int) (int64, error) {
	if offset != 0 || whence != 0 {
		return 0, errors.New("compressed can only seek to 0")
	}
	err1 := c.Close()
	n, err2 := c.src.Seek(0, 0)
	c.reset()
	if err1 != nil {
		return n, err1
	}
	return n, err2
}

func (c *compressed) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// reset restarts the compression loop.
func (c *compressed) reset() {
	var w *io.PipeWriter
	c.r, w = io.Pipe()
	c.wg.Add(1)
	go func() {
		// The compressor itself is not thread safe.
		defer c.wg.Done()
		compressor := isolated.GetCompressor(w)
		_, err := io.Copy(compressor, c.src)
		if err2 := compressor.Close(); err == nil {
			err = err2
		}
		w.CloseWithError(err)
	}()
}
