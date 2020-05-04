// Copyright 2020 The LUCI Authors.
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

package main

import (
	"flag"

	"golang.org/x/time/rate"

	"go.chromium.org/luci/server"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/bqexporter"
)

func main() {
	opts := bqexporter.DefaultOptions()
	flag.BoolVar(&opts.UseInsertIDs, "insert-ids", opts.UseInsertIDs,
		"Use InsertIDs when inserting data to BigQuery")
	flag.IntVar(&opts.MaxBatchSizeApprox, "max-batch-size-bytes-approx", opts.MaxBatchSizeApprox,
		"Maximum size of a batch in bytes, approximate")
	flag.IntVar(&opts.MaxBatchSizeApprox, "batch-buffer-size", opts.MaxBatchTotalSizeApprox,
		"Maximum cumulative size of batches, approximate")
	rateLimit := int(opts.RateLimit)
	flag.IntVar(&rateLimit, "rate-limit", rateLimit,
		"Maximum BigQuery request rate")

	internal.Main(func(srv *server.Server) error {
		opts.RateLimit = rate.Limit(rateLimit)
		bqexporter.InitServer(srv, opts)
		return nil
	})
}
