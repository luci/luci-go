// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"sync"

	"github.com/luci/luci-go/common/isolatedclient"
)

// Uploader uses an isolatedclient.Client to upload items to the server.
// All methods are safe for concurrent use.
type Uploader struct {
	ctx   context.Context
	svc   isolateService
	waitc chan bool // Used to cap concurrent uploads.
	wg    sync.WaitGroup

	errMu sync.Mutex
	err   error // The first error encountered, if any.
}

// NewUploader creates a new Uploader with the given isolated client.
// maxConcurrent controls maximum number of uploads to be in-flight at once.
// The provided context is used to make all requests to the isolate server.
func NewUploader(ctx context.Context, client *isolatedclient.Client, maxConcurrent int) *Uploader {
	return newUploader(ctx, client, maxConcurrent)
}

func newUploader(ctx context.Context, svc isolateService, maxConcurrent int) *Uploader {
	const concurrentUploads = 10
	return &Uploader{
		ctx:   ctx,
		svc:   svc,
		waitc: make(chan bool, maxConcurrent),
	}
}

// UploadBytes uploads an item held in-memory. UploadBytes does not block. If
// not-nil, the done func will be invoked on upload completion (both success
// and failure). The provided byte slice b must not be modified until the
// upload is completed.
func (u *Uploader) UploadBytes(name string, b []byte, ps *isolatedclient.PushState, done func()) {
	u.wg.Add(1)
	go u.upload(name, byteSource(b), ps, done)
}

// UploadFile uploads a file from disk. UploadFile does not block. If
// not-nil, the done func will be invoked on upload completion (both success
// and failure).
func (u *Uploader) UploadFile(item *Item, ps *isolatedclient.PushState, done func()) {
	u.wg.Add(1)
	go u.upload(item.RelPath, fileSource(item.Path), ps, done)
}

// Close waits for any pending uploads (and associated done callbacks) to
// complete, and returns the first encountered error if any.
// Uploader cannot be used once it is closed.
func (u *Uploader) Close() error {
	u.wg.Wait()
	close(u.waitc) // Sanity check that we don't do any more uploading.
	return u.err
}

func (u *Uploader) upload(name string, src isolatedclient.Source, ps *isolatedclient.PushState, done func()) {
	u.waitc <- true
	defer func() {
		<-u.waitc
		if done != nil {
			done()
		}
		u.wg.Done()
	}()

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

func (u *Uploader) getErr() error {
	u.errMu.Lock()
	defer u.errMu.Unlock()
	return u.err
}

func (u *Uploader) setErr(err error) {
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
