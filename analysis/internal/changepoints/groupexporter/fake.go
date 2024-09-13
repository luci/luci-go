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

package groupexporter

import (
	"context"

	bqp "go.chromium.org/luci/analysis/proto/bq"
)

// FakeClient represents a fake implementation of the test variant branch
// exporter, for testing.
type FakeClient struct {
	Insertions []*bqp.GroupedChangepointRow
}

// NewFakeClient creates a new FakeClient for exporting test variant branches.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		Insertions: []*bqp.GroupedChangepointRow{},
	}
}

// Insert inserts the given rows in BigQuery.
func (fc *FakeClient) Insert(ctx context.Context, rows []*bqp.GroupedChangepointRow) error {
	fc.Insertions = append(fc.Insertions, rows...)
	return nil
}
