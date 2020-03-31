// Copyright 2020 The LUCI Authors.
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
package sink

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"sync"

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type uploadTask struct {
	obj *storage.ObjectHandle
	art *sinkpb.Artifact
}

type gsChannel struct {
	bh       *storage.BucketHandle
	uploadCh chan *uploadTask
	drainCh  chan struct{}
	testGS   *testGStorage
}

func (gsc *gsChannel) init(ctx context.Context, cfg ServerConfig) error {
	bh := cfg.GStorage.Bucket(cfg.GSBucket)
	gsc.testGS = cfg.testGS
	if gsc.testGS == nil {
		// return an error from init() if the bucket is not accessible.
		if _, err := bh.Attrs(ctx); err != nil {
			return err
		}
	}
	gsc.bh = bh

	// TODO(ddoman): add metrics to track n of items buffered in uploadCh.
	gsc.uploadCh = make(chan *uploadTask, 1000)
	gsc.drainCh = make(chan struct{})

	go func() {
		logging.Infof(ctx, "starting the process loop for gs upload channel")
		defer close(gsc.drainCh)

		parallel.WorkPool(cfg.GSUploadMaxConcurrency, func(workC chan<- func() error) {
			var wg sync.WaitGroup
			for ut := gsc.findUploadTask(ctx); ut != nil; ut = gsc.findUploadTask(ctx) {
				// in-scope var for goroutine closure
				t := ut
				workC <- func() error {
					wg.Add(1)
					defer wg.Done()

					// capture and log the error here, so that they are logged
					// with an accurate enough timestamp.
					if err := gsc.upload(ctx, t); err != nil {
						fp := t.art.GetFilePath()
						if fp == "" {
							fp = fmt.Sprintf("contents(%d)", len(t.art.GetContents()))
						}
						logging.Errorf(
							ctx, "artifact (%q, %q), bucket (%q): %s",
							t.obj.ObjectName(), fp, t.obj.BucketName(), err,
						)
					}
					// ignore the error and return nil always so that no errors
					// are returned from parallel.WorkPool. Otherwise, all the
					// errors will be stacked together and returned as a single
					// MultiError.
					return nil
				}
			}
			logging.Infof(ctx, "waiting for all the ongoing tasks to be completed")
			wg.Wait()
		})
		logging.Infof(ctx, "terminating the processing loop for gs upload channel")
	}()
	return nil
}

func (gsc *gsChannel) close() {
	defer func() { recover() }()
	close(gsc.uploadCh)
}

func (gsc *gsChannel) closeAndDrain(ctx context.Context) {
	gsc.close()
	select {
	case <-ctx.Done():
	case <-gsc.drainCh:
	}
	return
}

func (gsc *gsChannel) findUploadTask(ctx context.Context) *uploadTask {
	select {
	case <-ctx.Done():
		logging.Infof(ctx, "Context cancelled; closing the the gs upload channel.")
		gsc.close()
	case ut, ok := <-gsc.uploadCh:
		if !ok {
			logging.Infof(ctx, "GS upload channel closed")
			break
		}
		return ut
	}
	return nil
}

func (gsc *gsChannel) uploadArtifact(name string, art *sinkpb.Artifact) (string, *timestamp.Timestamp) {
	obj := gsc.bh.Object(name)
	gsc.uploadCh <- &uploadTask{obj, art}
	// TODO(ddoman): return a URL expiration other than nil
	return fmt.Sprintf(
		"https://storage.googleapis.com/storage/v1/b/%s/o/%s",
		obj.BucketName(), url.QueryEscape(obj.ObjectName()),
	), nil
}

func (gsc *gsChannel) upload(ctx context.Context, ut *uploadTask) error {
	// * Notes
	// 1. "google.golang.org/api/storage/v1" and "cloud.google.com/go/storage" internally
	// retries an upload request if the error was recoverable.
	// 2. golang's poll.FD.Read, which os.Read invokes, handles and retries syscall.read
	// on transient errors, such as EINTR.
	//
	// If this function returns an error, it is neither recoverable nor transient.
	var err error
	var r io.Reader
	if p := ut.art.GetFilePath(); p != "" {
		// os.Open also retries on EINTR. If it returns an error, it's not
		// a transient error.
		r, err = os.Open(p)
		if err != nil {
			return err
		}
		defer r.(*os.File).Close()
	} else {
		r = bytes.NewBuffer(ut.art.GetContents())
	}

	var w io.WriteCloser
	if gsc.testGS == nil {
		w = ut.obj.NewWriter(ctx)
	} else {
		w = gsc.testGS.NewWriter(ut.obj)
	}
	if _, err := io.Copy(w, r); err != nil {
		w.Close()
		return err
	}
	return w.Close()
}
