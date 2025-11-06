// Copyright 2022 The LUCI Authors.
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

// Package gsutil contains utility functions for Google Storage.
package gsutil

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"go.chromium.org/luci/auth/scopes"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/server/auth"
)

type Client interface {
	GenerateSignedURL(ctx context.Context, bucket, object string, expiration time.Time) (string, error)
	Close()
}

// Client is a wrapper for cloud storage cloud which generated a Signed URL using project-scoped service account.
type StorageClient struct {
	gsClient    *storage.Client
	luciProject string
}

// SignURLOptsKey is the context key indicates using mocked storage.SignedURLOptions in tests.
var SignURLOptsKey = "used in tests only for setting a storage.SignedURLOptions"

// MockedGSClientKey is the context key indicates using mocked storage client in tests.
var MockedGSClientKey = "used in tests only for mocking the storage client"

// NewStorageClient create a new storage client.
func NewStorageClient(ctx context.Context, luciProject string) (Client, error) {
	if testClient, ok := ctx.Value(&MockedGSClientKey).(*MockClient); ok {
		testClient.luciProject = luciProject
		return testClient, nil
	}
	requiredScopes := []string{storage.ScopeReadOnly}
	requiredScopes = append(requiredScopes, scopes.CloudScopeSet()...)
	t, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(luciProject), auth.WithScopes(requiredScopes...))
	if err != nil {
		return nil, err
	}
	client, err := storage.NewClient(ctx, option.WithHTTPClient(&http.Client{Transport: t}))
	if err != nil {
		return nil, errors.Fmt("new storage client: %w", err)
	}
	return &StorageClient{
		gsClient:    client,
		luciProject: luciProject,
	}, nil
}

// GenerateSignedURL generates object signed URL.
func (c *StorageClient) GenerateSignedURL(ctx context.Context, bucket, object string, expiration time.Time) (string, error) {
	ctxOpts, ok := ctx.Value(&SignURLOptsKey).(*storage.SignedURLOptions)
	var opts *storage.SignedURLOptions
	if ok {
		opts = ctxOpts
	} else {
		opts = &storage.SignedURLOptions{
			Scheme:  storage.SigningSchemeV4,
			Method:  "GET",
			Expires: expiration,
			// Needs to provide GoogleAccessID, because it can't be automatically detected
			// when the client is authenticated with option.WithHTTPClient.
			// See: https://pkg.go.dev/cloud.google.com/go/storage#hdr-Credential_requirements_for_signing
			GoogleAccessID: fmt.Sprintf("%s-scoped@luci-project-accounts.iam.gserviceaccount.com", c.luciProject),
		}
	}

	// Use https://pkg.go.dev/cloud.google.com/go/storage#BucketHandle.SignedURL
	// to generate the signed URL.
	url, err := c.gsClient.Bucket(bucket).SignedURL(object, opts)
	if err != nil {
		return "", errors.Fmt("GenerateSignedURL(%q/%q): %v", bucket, object, err)
	}
	return url, nil
}

// Close releases resources associated with the client.
func (c *StorageClient) Close() {
	c.gsClient.Close()
}

// Split returns the bucket and filename components of the Path.
func Split(path string) (bucket string, filename string) {
	return gs.Path(path).Split()
}
