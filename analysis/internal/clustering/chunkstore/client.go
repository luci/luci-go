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

package chunkstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"

	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcmon"
	"go.chromium.org/luci/server/auth"

	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/pbutil"
)

// objectRe matches validly formed object IDs.
var objectRe = regexp.MustCompile(`^[0-9a-f]{32}$`)

// Client provides methods to store and retrieve chunks of test failures.
type Client struct {
	// client is the GCS client used to access chunks.
	client *storage.Client
	// bucket is the GCS bucket in which chunks are stored.
	bucket string
}

// NewClient initialises a new chunk storage client, that uses the specified
// GCS bucket as the backing store.
func NewClient(ctx context.Context, bucket string) (*Client, error) {
	// Credentials with Cloud scope.
	creds, err := auth.GetPerRPCCredentials(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, errors.Fmt("failed to get PerRPCCredentials: %w", err)
	}

	// Initialize the client.
	options := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithPerRPCCredentials(creds)),
		option.WithGRPCDialOption(grpc.WithStatsHandler(&grpcmon.ClientRPCStatsMonitor{})),
		option.WithScopes(storage.ScopeReadWrite),
	}
	cl, err := storage.NewClient(ctx, options...)

	if err != nil {
		return nil, errors.Fmt("failed to instantiate Cloud Storage client: %w", err)
	}
	return &Client{
		client: cl,
		bucket: bucket,
	}, nil
}

// Close releases resources associated with the client.
func (c *Client) Close() {
	c.client.Close()
}

// Put saves the given chunk to storage. If successful, it returns
// the randomly-assigned ID of the created object.
func (c *Client) Put(ctx context.Context, project string, content *cpb.Chunk) (objectID string, retErr error) {
	if err := pbutil.ValidateProject(project); err != nil {
		return "", err
	}
	b, err := proto.Marshal(content)
	if err != nil {
		return "", errors.Fmt("marhsalling chunk: %w", err)
	}
	objID, err := generateObjectID()
	if err != nil {
		return "", err
	}

	name := FileName(project, objID)
	doesNotExist := storage.Conditions{
		DoesNotExist: true,
	}
	// Only create the file if it does not exist. The risk of collision if
	// ID generation is working correctly is extremely remote so this mostly
	// defensive coding and a failsafe against bad randomness in ID generation.
	obj := c.client.Bucket(c.bucket).Object(name).If(doesNotExist)
	w := obj.NewWriter(ctx)
	defer func() {
		if err := w.Close(); err != nil && retErr == nil {
			retErr = errors.Fmt("closing object writer: %w", err)
		}
	}()

	// As the file is small (<8MB), set ChunkSize to object size to avoid
	// excessive memory usage, as per the documentation. Otherwise use
	// the default ChunkSize.
	if len(b) < 8*1024*1024 {
		w.ChunkSize = len(b)
	}
	w.ContentType = "application/x-protobuf"
	_, err = w.Write(b)
	if err != nil {
		return "", errors.Fmt("writing object %q: %w", name, err)
	}
	return objID, nil
}

// Get retrieves the chunk with the specified object ID and returns it.
func (c *Client) Get(ctx context.Context, project, objectID string) (chunk *cpb.Chunk, retErr error) {
	if err := pbutil.ValidateProject(project); err != nil {
		return nil, err
	}
	if err := validateObjectID(objectID); err != nil {
		return nil, err
	}
	name := FileName(project, objectID)
	obj := c.client.Bucket(c.bucket).Object(name)
	r, err := obj.NewReader(ctx)
	if err != nil {
		return nil, errors.Fmt("creating reader %q: %w", name, err)
	}
	defer func() {
		if err := r.Close(); err != nil && retErr == nil {
			retErr = errors.Fmt("closing object reader: %w", err)
		}
	}()

	// Allocate a buffer of the correct size and use io.ReadFull instead of
	// io.ReadAll to avoid needlessly reallocating slices.
	b := make([]byte, r.Attrs.Size)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, errors.Fmt("read object %q: %w", name, err)
	}
	content := &cpb.Chunk{}
	if err := proto.Unmarshal(b, content); err != nil {
		return nil, errors.Fmt("unmarshal chunk: %w", err)
	}
	return content, nil
}

func validateObjectID(id string) error {
	if !objectRe.MatchString(id) {
		return fmt.Errorf("object ID %q is not a valid", id)
	}
	return nil
}

// generateObjectID returns a random 128-bit object ID, encoded as
// 32 lowercase hexadecimal characters.
func generateObjectID() (string, error) {
	randomBytes := make([]byte, 16)
	_, err := rand.Read(randomBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(randomBytes), nil
}

// FileName returns the file path in GCS for the object with the
// given project and objectID. Exposed for testing only.
func FileName(project, objectID string) string {
	return fmt.Sprintf("/projects/%s/chunks/%s.binarypb", project, objectID)
}
