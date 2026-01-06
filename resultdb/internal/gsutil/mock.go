// Copyright 2025 The LUCI Authors.
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

package gsutil

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"
)

// MockClient is a mock implementation of the Client interface for testing.
type MockClient struct {
	luciProject string
	Content     map[string][]byte
}

// GenerateSignedURL implements the Client interface for MockClient.
func (m *MockClient) GenerateSignedURL(ctx context.Context, bucket, object string, expiration time.Time) (string, error) {
	return fmt.Sprintf("https://fake-signed-url/%s/%s?x-project=%s", bucket, object, m.luciProject), nil
}

// Close implements the Client interface for MockClient.
func (m *MockClient) Close() {
	// No-op for mock client.
}

// NewReader implements the Client interface for MockClient.
func (m *MockClient) NewReader(ctx context.Context, bucket, object string, offset int64) (io.ReadCloser, error) {
	key := fmt.Sprintf("gs://%s/%s", bucket, object)
	content, ok := m.Content[key]
	if !ok {
		return nil, fmt.Errorf("GCS object %q not found in mock content", key)
	}
	if offset > int64(len(content)) {
		offset = int64(len(content))
	}
	return io.NopCloser(bytes.NewReader(content[offset:])), nil
}
