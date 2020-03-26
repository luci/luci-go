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

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"
)

type gsRaw struct {
	bh *storage.BucketHandle
}

func newGSRaw(ctx context.Context, gsClient *storage.Client, bucket string) (*gsRaw, error) {
	bh := gsClient.Bucket(bucket)
	// check and returns an error, if the bucket is not accessible.
	if _, err := bh.Attrs(ctx); err != nil {
		return nil, err
	}
	return &gsRaw{bh}, nil
}

func (gs *gsRaw) uploadArtifact(ctx context.Context, name string, art *sinkpb.Artifact) error {
	// * Notes
	// 1. functions in "google.golang.org/api/storage/v1" and "cloud.google.com/go/storage"
	// automatically retries requests on recoverable errors.
	// e.g., 429 - Too Many Requests
	// 2. "google.golang.org/api/storage" creates and uses resumable upload sessions
	// to retry writes on recoverable errors.
	// 3. golang's poll.FD.Read, which os.Read invokes, handles and retries syscall.read
	// on transient errors, such as EINTR.
	//
	// If this function can return an error, it is either
	// - reads or writes failed due to non-recoverable errors, or
	// - the retry count for writes reached the limit.
	//
	// The parent call can either call this function again to restart the whole upload
	// session, or return an error.

	// check again if the bucket still exists.
	if _, err := gs.bh.Attrs(ctx); err != nil {
		return err
	}
	obj := gs.bh.Object(name)

	var input io.Reader
	if p := art.GetFilePath(); p != "" {
		// os.Open also retries on EINTR. If it returns an error, it's not a transient
		// error.
		input, err := os.Open(p)
		if err != nil {
			return err
		}
		defer input.Close()
	} else {
		input = bytes.NewReader(art.GetContents())
	}

	// start uploading
	w := obj.NewWriter(ctx)
	if _, err := io.Copy(w, input); err != nil {
		w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	// Make the artifact publicly accessible.
	if err := obj.ACL().Set(ctx, storage.AllUsers, storage.RoleReader); err != nil {
		return err
	}
	return nil
}

func (gs *gsRaw) fetchURL(name string, art *sinkpb.Artifact) (string, *timestamp.Timestamp) {
	// bh.Object() simply creates a struct without performing network operations.
	obj := gs.bh.Object(name)
	url := fmt.Sprintf(
		"https://storage.googleapis.com/storage/v1/b/%s/o/%s",
		obj.BucketName(), url.QueryEscape(obj.ObjectName()))

	// TODO(ddoman): return a URL expiration other than nil
	return url, nil
}
