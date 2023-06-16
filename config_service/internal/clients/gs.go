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
)

var gsClientCtxKey = "holds the Google Cloud Storage client"

// GsClient is an interface for interacting with Cloud Storage.
type GsClient interface {
	// UploadIf upload data to the bucket-to-object path if it meets the given
	// conditions. Return true, if the upload operation is made this time.
	UploadIf(ctx context.Context, bucket, object string, data []byte, conditions storage.Conditions) (bool, error)
	// Read reads data from the give bucket-to-object path.
	Read(ctx context.Context, bucket, object string) ([]byte, error)
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

// UploadIf upload data to the bucket-to-object path if it meets the given
// conditions. Return true, if the upload operation is made this time.
func (p *prodClient) UploadIf(ctx context.Context, bucket, object string, data []byte, conditions storage.Conditions) (bool, error) {
	var w io.WriteCloser
	if conditions == (storage.Conditions{}) {
		w = p.client.Bucket(bucket).Object(object).NewWriter(ctx)
	} else {
		w = p.client.Bucket(bucket).Object(object).If(conditions).NewWriter(ctx)
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
func (p *prodClient) Read(ctx context.Context, bucket, object string) ([]byte, error) {
	r, err := p.client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = r.Close()
	}()
	return io.ReadAll(r)
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
