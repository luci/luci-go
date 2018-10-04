// Copyright 2016 The LUCI Authors.
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

package archiver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sync"

	"go.chromium.org/luci/common/isolatedclient"
)

// Uploader uploads items to the server.
// It has a single implementation, *ConcurrentUploader. See ConcurrentUploader for method documentation.
type Uploader interface {
	Close() error
	Upload(name string, src isolatedclient.Source, ps *isolatedclient.PushState, done func())
	UploadBytes(name string, b []byte, ps *isolatedclient.PushState, done func())
	UploadFile(item *Item, ps *isolatedclient.PushState, done func())
}

// ConcurrentUploader uses an isolatedclient.Client to upload items to the server.
// All methods are safe for concurrent use.
type ConcurrentUploader struct {
	ctx   context.Context
	svc   isolateService
	waitc chan bool // Used to cap concurrent uploads.
	wg    sync.WaitGroup

	errMu sync.Mutex
	err   error // The first error encountered, if any.
}

// NewUploader creates a new ConcurrentUploader with the given isolated client.
// maxConcurrent controls maximum number of uploads to be in-flight at once.
// The provided context is used to make all requests to the isolate server.
func NewUploader(ctx context.Context, client *isolatedclient.Client, maxConcurrent int) *ConcurrentUploader {
	return newUploader(ctx, client, maxConcurrent)
}

func newUploader(ctx context.Context, svc isolateService, maxConcurrent int) *ConcurrentUploader {
	return &ConcurrentUploader{
		ctx:   ctx,
		svc:   svc,
		waitc: make(chan bool, maxConcurrent),
	}
}

// Upload uploads an item from an isolated.Source. Upload does not block. If
// not-nil, the done func will be invoked on upload completion (both success
// and failure).
func (u *ConcurrentUploader) Upload(name string, src isolatedclient.Source, ps *isolatedclient.PushState, done func()) {
	u.wg.Add(1)
	go u.upload(name, src, ps, done)
}

// UploadBytes uploads an item held in-memory. UploadBytes does not block. If
// not-nil, the done func will be invoked on upload completion (both success
// and failure). The provided byte slice b must not be modified until the
// upload is completed.
// TODO(djd): Consider using Upload directly and deleting UploadBytes.
func (u *ConcurrentUploader) UploadBytes(name string, b []byte, ps *isolatedclient.PushState, done func()) {
	u.wg.Add(1)
	go u.upload(name, byteSource(b), ps, done)
}

// UploadFile uploads a file from disk. UploadFile does not block. If
// not-nil, the done func will be invoked on upload completion (both success
// and failure).
// TODO(djd): Consider using Upload directly and deleting UploadFile.
func (u *ConcurrentUploader) UploadFile(item *Item, ps *isolatedclient.PushState, done func()) {
	u.wg.Add(1)
	go u.upload(item.RelPath, fileSource(item.Path), ps, done)
}

// Close waits for any pending uploads (and associated done callbacks) to
// complete, and returns the first encountered error if any.
// Uploader cannot be used once it is closed.
func (u *ConcurrentUploader) Close() error {
	u.wg.Wait()
	close(u.waitc) // Sanity check that we don't do any more uploading.
	return u.err
}

func (u *ConcurrentUploader) upload(name string, src isolatedclient.Source, ps *isolatedclient.PushState, done func()) {
	u.waitc <- true
	defer func() {
		<-u.waitc
	}()
	defer u.wg.Done()
	if done != nil {
		defer done()
	}

	// Bail out early if there already was an error.
	if u.getErr() != nil {
		log.Printf("WARNING dropped %q from Uploader", name)
		return
	}

	err := u.svc.Push(u.ctx, ps, src)
	if err != nil {
		u.setErr(fmt.Errorf("pushing %q: %v", name, err))
	}
}

func (u *ConcurrentUploader) getErr() error {
	u.errMu.Lock()
	defer u.errMu.Unlock()
	return u.err
}

func (u *ConcurrentUploader) setErr(err error) {
	u.errMu.Lock()
	defer u.errMu.Unlock()
	if u.err == nil {
		u.err = err
	}
}

func byteSource(b []byte) isolatedclient.Source {
	return func() (io.ReadCloser, error) {
		return ioutil.NopCloser(bytes.NewReader(b)), nil
	}
}

func fileSource(path string) isolatedclient.Source {
	return func() (io.ReadCloser, error) {
		return osOpen(path)
	}
}
