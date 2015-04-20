// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package isolatedclient

import (
	"io"
	"io/ioutil"
	"net/http"

	"github.com/luci/luci-go/client/internal/common"
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
	uploaded  bool
	finalized bool
}

// New returns a new IsolateServer client.
func New(url, namespace string) IsolateServer {
	return &isolateServer{
		url:       url,
		namespace: namespace,
	}
}

// Private details.

type isolateServer struct {
	url       string
	namespace string
}

func (i *isolateServer) ServerCapabilities() (*isolated.ServerCapabilities, error) {
	url := i.url + "/_ah/api/isolateservice/v1/server_details"
	out := &isolated.ServerCapabilities{}
	if _, err := common.PostJSON(nil, url, nil, out); err != nil {
		return nil, err
	}
	return out, nil
}

func (i *isolateServer) Contains(items []*isolated.DigestItem) ([]*PushState, error) {
	in := isolated.DigestCollection{Items: items}
	in.Namespace.Namespace = i.namespace
	data := &isolated.UrlCollection{}
	url := i.url + "/_ah/api/isolateservice/v1/preupload"
	if _, err := common.PostJSON(nil, url, in, data); err != nil {
		return nil, err
	}
	out := make([]*PushState, len(items))
	for _, e := range data.Items {
		index := int(e.Index)
		out[index] = &PushState{
			status: e,
		}
	}
	return out, nil
}

func (i *isolateServer) Push(state *PushState, src io.Reader) error {
	// This push operation may be a retry after failed finalization call below,
	// no need to reupload contents in that case.
	if !state.uploaded {
		// PUT file to uploadURL.
		if err := i.doPush(state, src); err != nil {
			return err
		}
		state.uploaded = true
	}

	// Optionally notify the server that it's done.
	if state.status.GSUploadURL != "" {
		// TODO(vadimsh): Calculate MD5 or CRC32C sum while uploading a file and
		// send it to isolated server. That way isolate server can verify that
		// the data safely reached Google Storage (GS provides MD5 and CRC32C of
		// stored files).
		in := isolated.FinalizeRequest{state.status.UploadTicket}
		url := i.url + "/_ah/api/isolateservice/v1/finalize_gs_upload"
		_, err := common.PostJSON(nil, url, in, nil)
		if err != nil {
			return err
		}
	}
	state.finalized = true
	return nil
}

func (i *isolateServer) doPush(state *PushState, src io.Reader) error {
	reader, writer := io.Pipe()
	defer reader.Close()
	compressor := isolated.GetCompressor(writer)

	go func() {
		io.Copy(compressor, src)
		compressor.Close()
		writer.Close()
	}()

	// DB upload.
	if state.status.GSUploadURL == "" {
		url := i.url + "/_ah/api/isolateservice/v1/store_inline"
		content, err := ioutil.ReadAll(reader)
		if err != nil {
			return err
		}
		in := &isolated.StorageRequest{state.status.UploadTicket, content}
		_, err = common.PostJSON(nil, url, in, nil)
		return err
	}

	// Upload to GCS.
	client := &http.Client{}
	request, err := http.NewRequest("PUT", state.status.GSUploadURL, reader)
	request.Header.Set("Content-Type", "application/octet-stream")
	// TODO(maruel): For relatively small file, set request.ContentLength so the
	// TCP connection can be reused.
	resp, err := client.Do(request)
	if err == nil {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}
	return err
}
