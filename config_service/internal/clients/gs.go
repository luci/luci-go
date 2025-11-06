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
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
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
	// Delete deletes the give object.
	Delete(ctx context.Context, bucket, object string) error
}

// prodClient implements GsClient and used in Prod env only.
type prodClient struct {
	client *storage.Client
}

// NewGsProdClient create a prodClient.
func NewGsProdClient(ctx context.Context) (GsClient, error) {
	ts, err := auth.GetTokenSource(ctx, auth.AsSelf, auth.WithScopes(scopes.CloudScopeSet()...))
	if err != nil {
		return nil, fmt.Errorf("failed to get OAuth2 token source: %w", err)
	}
	client, err := storage.NewClient(ctx, option.WithTokenSource(ts))
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
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == http.StatusPreconditionFailed {
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
	obj := p.client.Bucket(bucket).Object(object)
	err := retry.Retry(ctx, transient.Only(retry.Default),
		func() error {
			attr, err := obj.Attrs(ctx)
			switch {
			case err != nil:
				return err
			case !attr.CustomTime.IsZero() && clock.Now(ctx).Sub(attr.CustomTime) < 10*time.Minute:
				// If custom time is updated within last 10 minute, then skip updating.
				return nil
			}
			// Conditionally update the storage metadata and retry on pre-condition
			// failure. This is to mitigate the concurrent modification error occurs
			// when the same requester sends multiple validation requests that contain
			// the exact same config file. As a request, all the request processors
			// will contend to update the metadata of the same object.
			_, err = obj.
				If(storage.Conditions{MetagenerationMatch: attr.Metageneration}).
				Update(ctx, storage.ObjectAttrsToUpdate{
					CustomTime: clock.Now(ctx).UTC(),
				})
			var apiErr *googleapi.Error
			if errors.As(err, &apiErr) {
				switch apiErr.Code {
				case http.StatusConflict, http.StatusPreconditionFailed:
					// Tag precondition fail and conflict as transient to trigger a retry.
					return transient.Tag.Apply(err)
				}
			}
			return err
		}, func(err error, d time.Duration) {
			logging.Warningf(ctx, "got err: %s when touching the object. Retrying in %s", err, d)
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

// Delete deletes the give object.
func (p *prodClient) Delete(ctx context.Context, bucket, object string) error {
	return p.client.Bucket(bucket).Object(object).Delete(ctx)
}
