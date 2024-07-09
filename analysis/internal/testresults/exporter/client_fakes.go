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

package exporter

import (
	"context"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
)

// FakeClient represents a fake implementation of the test results
// exporter, for testing.
type FakeClient struct {
	// Insertions is the set of test results which were attempted
	// to be exported using the client.
	InsertionsByDestinationKey map[string][]*bqpb.TestResultRow
}

// NewFakeClient initialises a new client for exporting test results.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		InsertionsByDestinationKey: make(map[string][]*bqpb.TestResultRow),
	}
}

// Insert inserts the given rows.
func (fc *FakeClient) Insert(ctx context.Context, rows []*bqpb.TestResultRow, dest ExportDestination) error {
	fc.InsertionsByDestinationKey[dest.Key] = append(fc.InsertionsByDestinationKey[dest.Key], rows...)
	return nil
}
