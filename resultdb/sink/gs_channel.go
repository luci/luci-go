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

	"cloud.google.com/go/storage"
	"github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/logging"

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
}

func (gsc *gsChannel) init(ctx context.Context, cfg ServerConfig) error {
	bh := cfg.GStorage.Bucket(cfg.GSBucket)
	uploadFn := cfg.testUploadFn
	if uploadFn == nil {
		uploadFn = upload
		// check and returns an error, if the bucket is not accessible.
		if _, err := gsc.bh.Attrs(ctx); err != nil {
			return err
		}
	}
	gsc.bh = bh

	// TODO(ddoman): add metrics to track n of items buffered in uploadCh.
	gsc.uploadCh = make(chan *uploadTask, 1000)
	gsc.drainCh = make(chan struct{})
	go func() {
		logging.Infof(ctx, "Starting a goroutine for the upload channel")
		defer close(gsc.drainCh)

		for exit := false; !exit; {
			select {
			case <-ctx.Done():
				logging.Infof(ctx, "Context cancelled; finishing the upload channel.")
				exit = true
				break
			case ut, ok := <-gsc.uploadCh:
				if !ok {
					logging.Infof(ctx, "Channel closed.")
					exit = true
					break
				}

				var src io.Reader
				var err error
				if p := ut.art.GetFilePath(); p != "" {
					// os.Open also retries on EINTR. If it returns an error, it's not
					// a transient error.
					src, err = os.Open(p)
					if err != nil {
						logging.Errorf(ctx, "%q: %s", p, err)
						continue
					}
					defer src.(*os.File).Close()
				} else {
					src = bytes.NewReader(ut.art.GetContents())
				}
				if err := uploadFn(ctx, ut.obj, src); err != nil {
					logging.Errorf(
						ctx, "(%q, %q): %s", ut.obj.BucketName(), ut.obj.ObjectName(), err,
					)
				}
			}
		}
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

func (gsc *gsChannel) uploadArtifact(name string, art *sinkpb.Artifact) (string, *timestamp.Timestamp) {
	obj := gsc.bh.Object(name)
	if art == nil {
		panic("why nil")
	}
	gsc.uploadCh <- &uploadTask{obj, art}
	// TODO(ddoman): return a URL expiration other than nil
	return fmt.Sprintf(
		"https://storage.googleapis.com/storage/v1/b/%s/o/%s",
		obj.BucketName(), url.QueryEscape(obj.ObjectName()),
	), nil
}

func upload(ctx context.Context, dst *storage.ObjectHandle, src io.Reader) error {
	// * Notes
	// 1. "google.golang.org/api/storage/v1" and "cloud.google.com/go/storage" internally
	// retries an upload request if the error was recoverable.
	// 2. golang's poll.FD.Read, which os.Read invokes, handles and retries syscall.read
	// on transient errors, such as EINTR.
	//
	// If this function returns an error, it is neither recoverable nor transient.
	w := dst.NewWriter(ctx)
	if _, err := io.Copy(w, src); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return nil
}
