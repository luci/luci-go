// Copyright 2026 The LUCI Authors.
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

package clients

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/config_service/internal/common"
)

// UserDataHandle is a handle to data which is uploaded by the use during the
// 'verification' flow. The basic flow goes:
//
//   - User describes (by sha256) the data they would like to include in the
//     config set.
//   - The service replies with signed PUT URLs for any missing objects.
//   - The user uploads these and retries the verification request.
//   - The server then sends out verification requests to all services which
//     would potentially be impacted by the new set of configs.
//
// This handle allows minting the PUT url (for the user) or the GET url (for
// one of the affected services).
//
// It also allows Read exclusively for use in the legacy verification flow
// which sends data inline to downstream services.
type UserDataHandle interface {
	// Exists returns `true` if the UserDataHandle points to a GCS object which exists.
	Exists() bool

	// PutURL returns a signed URL which would allow a client to upload the
	// missing data.
	//
	// Returns an error if `Exists()` is true.
	PutURL(ctx context.Context) (string, error)

	// GetURL returns a signed URL which would allow a client to download the
	// data.
	//
	// Returns an error if `Exists()` is false.
	GetURL(ctx context.Context) (string, error)

	// Read allows reading the content of an existing user data object.
	//
	// Only used during the legacy verification workflow.
	Read(ctx context.Context) ([]byte, error)
}

type prodUserHandle struct {
	bucket     *storage.BucketHandle
	object     string
	generation int64
}

var _ UserDataHandle = (*prodUserHandle)(nil)

func (p *prodUserHandle) Exists() bool {
	return p.generation >= 0
}

func (p *prodUserHandle) PutURL(ctx context.Context) (string, error) {
	if p.Exists() {
		return "", fmt.Errorf("GsClient.UserDataHandle.PutURL: cannot mint PUT url for object gs://%s/%s: object exists", p.bucket.BucketName(), p.object)
	}
	opts, err := getSignedURLOptions(ctx, "PUT", -1, map[string]string{
		"Content-Encoding":            "gzip",
		"x-goog-content-length-range": fmt.Sprintf("0,%d", common.ConfigMaxSize),
	})
	if err != nil {
		return "", err
	}
	return p.bucket.SignedURL(p.object, opts)
}

func (p *prodUserHandle) GetURL(ctx context.Context) (string, error) {
	if !p.Exists() {
		return "", fmt.Errorf("GsClient.UserDataHandle.GetURL: cannot mint get GET url for object gs://%s/%s: %w", p.bucket.BucketName(), p.object, storage.ErrObjectNotExist)
	}

	opts, err := getSignedURLOptions(ctx, http.MethodGet, p.generation, nil)
	if err != nil {
		return "", err
	}
	return p.bucket.SignedURL(p.object, opts)
}

func (p *prodUserHandle) Read(ctx context.Context) ([]byte, error) {
	if !p.Exists() {
		return nil, fmt.Errorf("GsClient.UserDataHandle.Read: cannot read object gs://%s/%s: %w", p.bucket.BucketName(), p.object, storage.ErrObjectNotExist)
	}

	obj := p.bucket.Object(p.object).Generation(p.generation)
	r, err := newReader(ctx, obj)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
