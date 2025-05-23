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

package exporter

import (
	"context"

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
)

// FakeClient is a fake implementation of Exporter for testing.
type FakeClient struct {
	Insertion *bqpb.AntsInvocationRow
}

// NewFakeClient creates a new FakeClient for exporting ants invocation.
func NewFakeClient() *FakeClient {
	return &FakeClient{}
}

// Insert captures the rows that would have been exported.
func (fc *FakeClient) Insert(ctx context.Context, row *bqpb.AntsInvocationRow) error {
	fc.Insertion = row
	return nil
}
