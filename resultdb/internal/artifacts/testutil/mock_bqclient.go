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

// Package testutil provides utility functions for testing with artifacts package.
package testutil

import (
	"context"

	"go.chromium.org/luci/resultdb/internal/artifacts"
)

type ReadArtifactGroupsFunc func(ctx context.Context, opts artifacts.ReadArtifactGroupsOpts) (groups []*artifacts.ArtifactGroup, nextPageToken string, err error)
type ReadTestArtifactsFunc func(ctx context.Context, opts artifacts.ReadTestArtifactsOpts) (rows []*artifacts.MatchingArtifact, nextPageToken string, err error)

// MockBQClient is a mock implementation of the BQClient interface.
type MockBQClient struct {
	ReadArtifactGroupsFunc ReadArtifactGroupsFunc
	ReadTestArtifactsFunc  ReadTestArtifactsFunc
}

// ReadArtifactGroups implements the BQClient interface.
func (m *MockBQClient) ReadArtifactGroups(ctx context.Context, opts artifacts.ReadArtifactGroupsOpts) (groups []*artifacts.ArtifactGroup, nextPageToken string, err error) {
	if m.ReadArtifactGroupsFunc != nil {
		return m.ReadArtifactGroupsFunc(ctx, opts)
	}
	return nil, "", nil
}

// ReadTestArtifacts implements the BQClient interface.
func (m *MockBQClient) ReadTestArtifacts(ctx context.Context, opts artifacts.ReadTestArtifactsOpts) (groups []*artifacts.MatchingArtifact, nextPageToken string, err error) {
	if m.ReadTestArtifactsFunc != nil {
		return m.ReadTestArtifactsFunc(ctx, opts)
	}
	return nil, "", nil
}

// NewMockBQClient creates a new MockBQClient with the given ReadTestArtifactGroupsFunc.
func NewMockBQClient(readTestArtifactGroupsFunc ReadArtifactGroupsFunc, readTestArtifactsFunc ReadTestArtifactsFunc) *MockBQClient {
	return &MockBQClient{
		ReadArtifactGroupsFunc: readTestArtifactGroupsFunc,
		ReadTestArtifactsFunc:  readTestArtifactsFunc,
	}
}
