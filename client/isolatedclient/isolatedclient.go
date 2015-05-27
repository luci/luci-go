// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"

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
	Push(state *PushState, src io.Reader) error
}

// PushState is per-item state passed from IsolateServer.Contains() to
// IsolateServer.Push().
//
// Its content is implementation specific.
type PushState struct {
	status    isolated.PreuploadStatus
	size      int64
	uploaded  bool
	finalized bool
}

// New returns a new IsolateServer client.
func New(host, namespace string) IsolateServer {
	i := &isolateServer{
		url:       strings.TrimRight(host, "/"),
		namespace: namespace,
	}
	tracer.NewTID(i, nil, i.url)
	return i
}

// Private details.

type isolateServer struct {
	url       string
	namespace string
}

func (i *isolateServer) postJSON(resource string, in, out interface{}) error {
	if len(resource) == 0 || resource[0] != '/' {
		return errors.New("resource must start with '/'")
	}
	_, err := lhttp.PostJSON(retry.Default, http.DefaultClient, i.url+resource, in, out)
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
	end := tracer.Span(i, "contains", strconv.Itoa(len(items)), nil)
	defer func() { end(tracer.Args{"err": err}) }()
	in := isolated.DigestCollection{Items: items}
	in.Namespace.Namespace = i.namespace
	data := &isolated.UrlCollection{}
	if err = i.postJSON("/_ah/api/isolateservice/v1/preupload", in, data); err != nil {
		return nil, err
	}
	out = make([]*PushState, len(items))
	for _, e := range data.Items {
		index := int(e.Index)
		out[index] = &PushState{
			status: e,
			size:   items[index].Size,
		}
	}
	return out, nil
}

func (i *isolateServer) Push(state *PushState, src io.Reader) (err error) {
	// This push operation may be a retry after failed finalization call below,
	// no need to reupload contents in that case.
	if !state.uploaded {
		// PUT file to uploadURL.
		if err = i.doPush(state, src); err != nil {
			return
		}
		state.uploaded = true
	}

	// Optionally notify the server that it's done.
	if state.status.GSUploadURL != "" {
		end := tracer.Span(i, "finalize", "finalize", nil)
		defer func() { end(tracer.Args{"err": err}) }()
		// TODO(vadimsh): Calculate MD5 or CRC32C sum while uploading a file and
		// send it to isolated server. That way isolate server can verify that
		// the data safely reached Google Storage (GS provides MD5 and CRC32C of
		// stored files).
		in := isolated.FinalizeRequest{state.status.UploadTicket}
		if err = i.postJSON("/_ah/api/isolateservice/v1/finalize_gs_upload", in, nil); err != nil {
			return
		}
	}
	state.finalized = true
	return
}

func (i *isolateServer) doPush(state *PushState, src io.Reader) (err error) {
	end := tracer.Span(i, "push", strconv.FormatInt(state.size, 10), nil)
	defer func() { end(tracer.Args{"err": err}) }()
	reader, writer := io.Pipe()
	defer reader.Close()
	compressor := isolated.GetCompressor(writer)
	c := make(chan error)
	go func() {
		_, err2 := io.Copy(compressor, src)
		if err3 := compressor.Close(); err2 == nil {
			err2 = err3
		}
		_ = writer.Close()
		c <- err2
	}()
	defer func() {
		err4 := <-c
		if err == nil {
			err = err4
		}
	}()

	// DB upload.
	if state.status.GSUploadURL == "" {
		content, err2 := ioutil.ReadAll(reader)
		if err2 != nil {
			return err2
		}
		in := &isolated.StorageRequest{state.status.UploadTicket, content}
		return i.postJSON("/_ah/api/isolateservice/v1/store_inline", in, nil)
	}

	// Upload to GCS.
	client := &http.Client{}
	request, err5 := http.NewRequest("PUT", state.status.GSUploadURL, reader)
	if err5 != nil {
		return err5
	}
	request.Header.Set("Content-Type", "application/octet-stream")
	// TODO(maruel): For relatively small file, set request.ContentLength so the
	// TCP connection can be reused.
	resp, err6 := client.Do(request)
	if err6 != nil {
		return err6
	}
	_, err = io.Copy(ioutil.Discard, resp.Body)
	_ = resp.Body.Close()
	tracer.CounterAdd(i, "isolateserver", "upload", float64(state.size))
	return
}
