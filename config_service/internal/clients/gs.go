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
	"net/url"
	"sort"
	"strconv"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/auth"
)

const (
	// SignedURLExpireDur defines how long the signed url will be active.
	SignedURLExpireDur = 10 * time.Minute
)

var (
	// ErrTooLarge is the core error returned when a decompressed GCS object
	// exceeds common.ConfigMaxSize.
	ErrTooLarge = errors.New("gcs object too large")

	// ErrInvalidGzip is returned when a GCS object is not valid gzip data.
	ErrInvalidGzip = errors.New("gcs object has invalid gzip stream")

	gsClientCtxKey = "holds the Google Cloud Storage client"
)

// GsClient is an interface for interacting with Cloud Storage.
type GsClient interface {
	// UploadIfMissing uploads data to the bucket-to-object path if it does not
	// exist.
	//
	// `attrsModifyFn` is allowed to mutate the ObjectAttrs prior to the upload.
	//
	// Returns true, if the upload occurred.
	UploadIfMissing(ctx context.Context, bucket, object string, data []byte, attrsModifyFn func(*storage.ObjectAttrs)) (bool, error)

	// Read reads and gunzips data from the given bucket-to-object path.
	//
	// May return an error wrapping [ErrTooLarge] or [ErrInvalidGzip].
	Read(ctx context.Context, bucket, object string) ([]byte, error)

	// Touch updates the 'custom time' of the object to the current timestamp.
	//
	// Returns storage.ErrObjectNotExist if object is not found.
	Touch(ctx context.Context, bucket, object string) error

	// SignedURL is used to generate a signed url for a given GCS object.
	SignedURL(bucket, object string, opts *storage.SignedURLOptions) (string, error)

	// Delete deletes the give object.
	Delete(ctx context.Context, bucket, object string) error

	// ServiceDataHandle handles the data ingested from committed-to-git
	// configuration via UploadIfMissing.
	ServiceDataHandle(bucket, object string) ServiceDataHandle
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

// Read reads and gunzips data from the given bucket-to-object path.
func (p *prodClient) Read(ctx context.Context, bucket, object string) (ret []byte, err error) {
	r, err := newReader(ctx, p.client.Bucket(bucket).Object(object))
	if err != nil {
		return nil, err
	}
	defer r.Close()
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
			if apiErr, ok := errors.AsType[*googleapi.Error](err); ok {
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

func (p *prodClient) ServiceDataHandle(bucket, object string) ServiceDataHandle {
	return &prodServiceHandle{p.client.Bucket(bucket), object, -1}
}

// getSignedURLOptions returns a [storage.SignedURLOptions] suitable for use
// with [GsClient.SignedURL].
//
// This uses [auth.GetSigner] to bind the returned options to the current
// service account.
func getSignedURLOptions(ctx context.Context, method string, generation int64, headers map[string]string) (*storage.SignedURLOptions, error) {
	signer := auth.GetSigner(ctx)
	info, err := signer.ServiceInfo(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get service info: %w", err)
	}
	expireAt := clock.Now(ctx).Add(SignedURLExpireDur)
	var hdrs []string
	if len(headers) > 0 {
		hdrs = make([]string, 0, len(headers))
		for k, v := range headers {
			hdrs = append(hdrs, fmt.Sprintf("%s:%s", k, v))
		}
		sort.Strings(hdrs)
	}
	var qParms url.Values
	if generation >= 0 {
		qParms = url.Values{}
		qParms.Set("generation", strconv.FormatInt(generation, 10))
	}
	return &storage.SignedURLOptions{
		GoogleAccessID: info.ServiceAccountName,
		SignBytes: func(b []byte) ([]byte, error) {
			_, signedBytes, err := signer.SignBytes(ctx, b)
			return signedBytes, err
		},
		Scheme:          storage.SigningSchemeV4,
		Method:          method,
		Expires:         expireAt,
		Headers:         hdrs,
		QueryParameters: qParms,
	}, nil
}

// CreateSignedURLs create signed urls for the given GS paths.
//
// Uses the service account the service currently runs as to authorize
// and sign the urls.
func CreateSignedURLs(ctx context.Context, gsClient GsClient, paths []gs.Path, method string, headers map[string]string) ([]string, error) {
	opts, err := getSignedURLOptions(ctx, method, -1, headers)
	if err != nil {
		return nil, err
	}
	ret := make([]string, len(paths))
	for i, path := range paths {
		bucket, object := path.Split()
		ret[i], err = gsClient.SignedURL(bucket, object, opts)
		if err != nil {
			return nil, err
		}
	}
	return ret, nil
}
