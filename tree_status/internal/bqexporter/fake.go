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

// Package bqexporter exports status data to BigQuery.
package bqexporter

import (
	"context"
	"time"

	bqpb "go.chromium.org/luci/tree_status/proto/bq"
)

// FakeClient represents a fake implementation of the test variant branch
// exporter, for testing.
type FakeClient struct {
	Insertions []*bqpb.StatusRow
}

// NewFakeClient creates a new FakeClient for exporting test variant branches.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		Insertions: []*bqpb.StatusRow{},
	}
}

func (fc *FakeClient) EnsureSchema(ctx context.Context) error {
	return nil
}

func (fc *FakeClient) InsertStatusRows(ctx context.Context, rows []*bqpb.StatusRow) error {
	fc.Insertions = append(fc.Insertions, rows...)
	return nil
}

func (fc *FakeClient) ReadMostRecentCreateTime(ctx context.Context) (time.Time, error) {
	return time.Unix(100, 0).UTC(), nil
}
