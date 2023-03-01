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

package testvariantbqexporter

import (
	"context"
	"net/http"

	"cloud.google.com/go/bigquery"
	"golang.org/x/sync/semaphore"
	"golang.org/x/time/rate"
	"google.golang.org/api/option"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/analysis/internal/bqutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

const (
	maxBatchRowCount  = 1000
	rateLimit         = 100
	maxBatchTotalSize = 200 * 1000 * 1000 // instance memory limit is 512 MB.
	rowSizeApprox     = 2000
)

// schemaApplyer ensures BQ schema matches the row proto definitions.
var schemaApplyer = bq.NewSchemaApplyer(bq.RegisterSchemaApplyerCache(50))

// Options specifies the requirements of the bq export.
type Options struct {
	Realm        string
	CloudProject string
	Dataset      string
	Table        string
	Predicate    *atvpb.Predicate
	TimeRange    *pb.TimeRange
}

// BQExporter exports test variant rows to the dedicated table.
type BQExporter struct {
	options *Options

	client *bigquery.Client

	// putLimiter limits the rate of bigquery.Inserter.Put calls.
	putLimiter *rate.Limiter

	// batchSem limits the number of batches we hold in memory at a time.
	batchSem *semaphore.Weighted
}

func CreateBQExporter(options *Options) *BQExporter {
	if options.Predicate == nil {
		options.Predicate = &atvpb.Predicate{}
	}
	return &BQExporter{
		options:    options,
		putLimiter: rate.NewLimiter(rateLimit, 1),
		batchSem:   semaphore.NewWeighted(int64(maxBatchTotalSize / rowSizeApprox / maxBatchRowCount)),
	}
}

func (b *BQExporter) createBQClient(ctx context.Context) error {
	project, _ := realms.Split(b.options.Realm)
	tr, err := auth.GetRPCTransport(ctx, auth.AsProject, auth.WithProject(project), auth.WithScopes(bigquery.Scope))
	if err != nil {
		return err
	}

	b.client, err = bigquery.NewClient(ctx, b.options.CloudProject, option.WithHTTPClient(&http.Client{
		Transport: tr,
	}))
	return err
}

// ExportRows test variants in batch.
func (b *BQExporter) ExportRows(ctx context.Context) error {
	err := b.createBQClient(ctx)
	if err != nil {
		return err
	}

	table := b.client.Dataset(b.options.Dataset).Table(b.options.Table)
	if err = schemaApplyer.EnsureTable(ctx, table, tableMetadata); err != nil {
		return errors.Annotate(err, "ensuring test variant table in dataset %q", b.options.Dataset).Err()
	}

	inserter := bqutil.NewInserter(table, maxBatchRowCount)
	if err = b.exportTestVariantRows(ctx, inserter); err != nil {
		return errors.Annotate(err, "export test variant rows").Err()
	}

	return nil
}
