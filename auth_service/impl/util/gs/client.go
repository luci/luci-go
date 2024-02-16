// Copyright 2024 The LUCI Authors.
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

package gs

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
)

const (
	// The chunk size to use when uploading to GS.
	//
	// Must be under 10 MB to avoid hitting GAE URL Fetch request size
	// limits and must be larger than 262144 bytes to satisfy GCS
	// requirements. Recommended by GCS to be a multiple of 262144.
	maxChunkSize = 262144 * 34 // ~= 9 MB
)

// Client abstracts functionality to connect with and use Google
// Storage.
//
// Non-production implementations are used primarily for testing.
type Client interface {
	// Close closes the connection to Google Storage.
	Close() error

	// WriteFile writes the given data to the GS path with the object ACLs
	// provided.
	WriteFile(ctx context.Context, objectPath, contentType string, data []byte, acls []storage.ACLRule) error
}

type gsClient struct {
	baseClient *storage.Client
}

// NewGSClient creates a new production Google Storage client; i.e. this
// client is actually Google Storage, not a mock.
func NewGSClient(ctx context.Context) (*gsClient, error) {
	logging.Debugf(ctx, "Creating new Google Storage client")
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Annotate(err, "aborting - failed setting up authenticated requests to Google Storage").Err()
	}

	var opts []option.ClientOption
	if tr != nil {
		opts = []option.ClientOption{
			option.WithHTTPClient(&http.Client{Transport: tr}),
		}
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, errors.Annotate(err, "failed to create Google Storage client").Err()
	}

	return &gsClient{
		baseClient: client,
	}, nil
}

func (c *gsClient) Close() error {
	if c.baseClient != nil {
		err := c.baseClient.Close()
		if err != nil {
			return err
		}
		c.baseClient = nil
	}
	return nil
}

func (c *gsClient) WriteFile(ctx context.Context, objectPath, contentType string, data []byte, acls []storage.ACLRule) (retErr error) {
	if c.baseClient == nil {
		return fmt.Errorf("aborting - no Google Storage client")
	}

	bucket, name, found := strings.Cut(objectPath, "/")
	if !found {
		return fmt.Errorf("aborting - invalid object path %s", objectPath)
	}

	writer := c.baseClient.Bucket(bucket).Object(name).NewWriter(ctx)
	defer func() {
		err := writer.Close()
		if retErr == nil && err != nil {
			retErr = errors.Annotate(err, "error uploading %s", objectPath).Err()
			return
		}

		logging.Debugf(ctx, "GS write successful.\nAttributes for %s: %+v",
			objectPath, writer.Attrs())
	}()

	writer.ContentType = contentType
	writer.ACL = acls
	writer.ChunkSize = maxChunkSize
	writer.ChunkRetryDeadline = 30 * time.Second

	if _, err := writer.Write(data); err != nil {
		return errors.Annotate(err, "error uploading %s", objectPath).Err()
	}

	return nil
}
