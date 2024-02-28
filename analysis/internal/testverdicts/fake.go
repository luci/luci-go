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

package testverdicts

import (
	"context"

	bqp "go.chromium.org/luci/analysis/proto/bq"
)

// FakeClient represents a fake implementation of the test verdicts
// exporter, for testing.
type FakeClient struct {
	// Insertions is the set of test verdicts which were attempted
	// to be exported using the client.
	Insertions []*bqp.TestVerdictRow
}

// NewFakeClient initialises a new client for exporting test verdicts.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		Insertions: []*bqp.TestVerdictRow{},
	}
}

// Insert inserts the given rows.
func (fc *FakeClient) Insert(ctx context.Context, rows []*bqp.TestVerdictRow) error {
	fc.Insertions = append(fc.Insertions, rows...)
	return nil
}

// FakeReadClient represents a fake implementation of the client to read test verdicts
// from BigQuery, for testing.
type FakeReadClient struct {
	CommitsWithVerdicts []*CommitWithVerdicts
}

// ReadTestVerdictsPerSourcePosition reads test verdicts per source position.
func (f *FakeReadClient) ReadTestVerdictsPerSourcePosition(ctx context.Context, options ReadTestVerdictsPerSourcePositionOptions) ([]*CommitWithVerdicts, error) {
	return f.CommitsWithVerdicts, nil
}
