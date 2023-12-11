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

package exporter

import (
	"context"
	"time"

	"cloud.google.com/go/bigquery"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
)

// FakeClient represents a fake implementation of the failure association rules BigQuery
// exporter, for testing.
type FakeClient struct {
	Insertions []*bqpb.FailureAssociationRulesHistoryRow

	lastUpdate bigquery.NullTimestamp
	errs       []error
}

// NewFakeClient creates a new FakeClient for exporting failure association rules.
func NewFakeClient() *FakeClient {
	return &FakeClient{
		Insertions: make([]*bqpb.FailureAssociationRulesHistoryRow, 0),
		lastUpdate: bigquery.NullTimestamp{},
		errs:       []error{},
	}
}

// LastUpdate set the output of NewestLastUpdated in future calls.
func (fc *FakeClient) LastUpdate(t time.Time) *FakeClient {
	fc.lastUpdate = bigquery.NullTimestamp{
		Timestamp: t,
		Valid:     true,
	}
	return fc
}

// Errs set errors return in the future calls in sequence.
func (fc *FakeClient) Errs(errs []error) *FakeClient {
	fc.errs = errs
	return fc
}

// Insert inserts the given rows in BigQuery.
func (fc *FakeClient) Insert(ctx context.Context, rows []*bqpb.FailureAssociationRulesHistoryRow) error {
	var e error
	if len(fc.errs) > 0 {
		e = fc.errs[0]
		fc.errs = fc.errs[1:]
	}
	fc.Insertions = append(fc.Insertions, rows...)
	return e
}

// NewestLastUpdated get the largest value in the lastUpdated field from the BigQuery table.
func (fc *FakeClient) NewestLastUpdated(ctx context.Context) (bigquery.NullTimestamp, error) {
	var e error
	if len(fc.errs) > 0 {
		e = fc.errs[0]
		fc.errs = fc.errs[1:]
	}
	return fc.lastUpdate, e
}
