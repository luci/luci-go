// Copyright 2023 The LUCI Authors.
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
	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/common/clock"
)

var gsClientCtxKey = "holds the Google Cloud Storage client"

// GsClient is an interface for interacting with Cloud Storage.
type GsClient interface {
	// UploadIfMissing upload data with attrs to the bucket-to-object path if it
	// does not exist. Return true, if the upload operation is made this time.
	UploadIfMissing(ctx context.Context, bucket, object string, data []byte, attrsModifyFn func(*storage.ObjectAttrs)) (bool, error)
	// Read reads data from the give bucket-to-object path.
	// If decompressive is true, it will read with decompressive transcoding:
	// https://cloud.google.com/storage/docs/transcoding#decompressive_transcoding
	Read(ctx context.Context, bucket, object string, decompressive bool) ([]byte, error)
	// Touch updates the custom time of the object to the current timestamp.
	//
	// Returns storage.ErrObjectNotExist if object is not found.
	Touch(ctx context.Context, bucket, object string) error
	// SignedURL is used to generate a signed url for a given GCS object.
	SignedURL(bucket, object string, opts *storage.SignedURLOptions) (string, error)
}

// prodClient implements GsClient and used in Prod env only.
type prodClient struct {
	client *storage.Client
}

// NewGsProdClient create a prodClient.
func NewGsProdClient(ctx context.Context) (GsClient, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	return &prodClient{
		client: client,
	}, nil
}

// WithGsClient returns a new context with the given GsClient.
func WithGsClient(ctx context.Context, client GsClient) context.Context {
	return context.WithValue(ctx, &gsClientCtxKey, client)
}

// GetGsClient returns the GsClient installed in the current context.
func GetGsClient(ctx context.Context) GsClient {
	return ctx.Value(&gsClientCtxKey).(GsClient)
}

// UploadIfMissing upload data with attrs to the bucket-to-object path if it
// does not exist. Return true, if the upload operation is made this time.
func (p *prodClient) UploadIfMissing(ctx context.Context, bucket, object string, data []byte, attrsModifyFn func(*storage.ObjectAttrs)) (bool, error) {
	w := p.client.Bucket(bucket).Object(object).If(storage.Conditions{DoesNotExist: true}).NewWriter(ctx)
	if attrsModifyFn != nil {
		attrsModifyFn(&w.ObjectAttrs)
	}

	if _, err := w.Write(data); err != nil {
		return false, err
	}
	if err := w.Close(); err != nil {
		if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusPreconditionFailed {
			// The provided condition has already meet, no uploads are done.
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Read reads data from the give bucket-to-object path.
// If decompressive is true, it will read with decompressive transcoding:
// https://cloud.google.com/storage/docs/transcoding#decompressive_transcoding
func (p *prodClient) Read(ctx context.Context, bucket, object string, decompressive bool) ([]byte, error) {
	r, err := p.client.Bucket(bucket).Object(object).ReadCompressed(!decompressive).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = r.Close()
	}()
	return io.ReadAll(r)
}

// Touch updates the custom time of the object to the current timestamp.
//
// Returns storage.ErrObjectNotExist if object is not found.
func (p *prodClient) Touch(ctx context.Context, bucket, object string) error {
	_, err := p.client.Bucket(bucket).Object(object).Update(ctx, storage.ObjectAttrsToUpdate{
		CustomTime: clock.Now(ctx).UTC(),
	})
	return err
}

// SignedURL is used to generate a signed url for a given GCS object.
func (p *prodClient) SignedURL(bucket, object string, opts *storage.SignedURLOptions) (string, error) {
	// Use https://pkg.go.dev/cloud.google.com/go/storage#BucketHandle.SignedURL
	// to generate the signed URL.
	url, err := p.client.Bucket(bucket).SignedURL(object, opts)
	if err != nil {
		return "", err
	}
	return url, nil
}
