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
	"time"

	"cloud.google.com/go/storage"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
)

type Key string

// GenerateSignedURL generates object signed URL.
func GenerateSignedURL(ctx context.Context, gsClient *storage.Client, bucket, object string,
	expiration time.Time, opts *storage.SignedURLOptions) (string, error) {

	// For passing GoogleAcessId used in testing
	if opts == nil {
		// GoogleAccessID & privateKey don't need to be provided as they will be
		// automatically detected in GCE.
		// See: https://pkg.go.dev/cloud.google.com/go/storage#hdr-Credential_requirements_for_signing
		opts = &storage.SignedURLOptions{
			Scheme:  storage.SigningSchemeV4,
			Method:  "GET",
			Expires: expiration,
		}
	}

	// Use https://pkg.go.dev/cloud.google.com/go/storage#BucketHandle.SignedURL
	// to generate the signed URL.
	url, err := gsClient.Bucket(bucket).SignedURL(object, opts)
	if err != nil {
		return "", errors.Fmt("GenerateSignedURL(%q/%q): %v", bucket, object, err)
	}

	return url, nil
}

// Split returns the bucket and filename components of the Path.
func Split(path string) (bucket string, filename string) {
	return gs.Path(path).Split()
}
