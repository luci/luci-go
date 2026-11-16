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
	"io"
	"net/http"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/common/errors"
)

// ServiceDataHandle is a handle to a GCS object which is written by the LUCI
// Config service itself (e.g. ingested from Git) via
// [GsClient.UploadIfMissing].
type ServiceDataHandle interface {
	// Read allows reading the content of an existing object.
	Read(ctx context.Context) ([]byte, error)

	// Delete deletes the given object.
	Delete(ctx context.Context) error

	// GetURL returns a signed URL which would allow a client to download the
	// data.
	GetURL(ctx context.Context) (string, error)
}

type prodServiceHandle struct {
	bucket     *storage.BucketHandle
	object     string
	generation int64
}

var _ ServiceDataHandle = (*prodServiceHandle)(nil)

func (p *prodServiceHandle) Read(ctx context.Context) ([]byte, error) {
	obj := p.bucket.Object(p.object)
	if p.generation >= 0 {
		obj = obj.Generation(p.generation)
	}
	r, err := newReader(ctx, obj)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if p.generation < 0 {
		p.generation = r.Attrs().Generation
	}
	return io.ReadAll(r)
}

func (p *prodServiceHandle) Delete(ctx context.Context) error {
	if err := p.bucket.Object(p.object).Delete(ctx); !errors.Is(err, storage.ErrObjectNotExist) {
		return err
	}
	return nil
}

func (p *prodServiceHandle) GetURL(ctx context.Context) (string, error) {
	opts, err := getSignedURLOptions(ctx, http.MethodGet, p.generation, nil)
	if err != nil {
		return "", err
	}
	return p.bucket.SignedURL(p.object, opts)
}
