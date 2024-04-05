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

package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/x/beamx"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/signals"

	"go.chromium.org/luci/logdog/appengine/coordinator"
	"go.chromium.org/luci/logdog/dataflow/dsutils"
	"go.chromium.org/luci/logdog/dataflow/logstream"
)

var (
	cloudProject = flag.String("cloud-project", "", "Logdog cloud project.")

	commit = flag.Bool("commit", false, "Whether the expiry timestamp update should be applied to the datastore.")

	// backfillWorkers controls the number of go workers in a backfill beam
	// worker.
	backfillWorkers = flag.Int(
		"backfill-workers",
		4,
		"The number of go workers in a backfill beam worker.",
	)
)

func main() {
	flag.Parse()

	// Must be called after `flag.Parse()` because this should be called after
	// flags processing.
	//
	// Must be called before any flag value is accessed. Otherwise the workers
	// will fail somehow.
	beam.Init()

	ctx := gologger.StdConfig.Use(context.Background())
	ctx, cancel := context.WithCancel(ctx)
	signals.HandleInterrupt(cancel)

	if *cloudProject == "" {
		errors.Log(ctx, errors.Reason("-cloud-project is required").Err())
		os.Exit(2)
		return
	}

	if err := run(ctx); err != nil {
		errors.Log(ctx, err)
		os.Exit(1)
		return
	}
}

func run(ctx context.Context) error {
	p := beam.NewPipeline()
	s := p.Root()

	backfillOpts := logstream.BackfillOptions{
		DryRun:    !*commit,
		BatchSize: 256,
		Workers:   *backfillWorkers,
		// Entities created this year all have the expire at property populated.
		// Only processing old entities also let us commit updates safely without
		// using transaction.
		SkipCreatedAfter: time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC),
		Expiry:           coordinator.LogStreamExpiry,
		RetryCount:       5,
	}

	readOpts := dsutils.ReadOptions{
		OutputBatchSize: backfillOpts.BatchSize * backfillOpts.Workers * 16,
		// There are 64, but 10 should be more than enough.
		// Can split to at most 16^10 + 1 splits.
		HexPrefixLength:   10,
		MinEstimatedCount: 1000,
		// Initially, we use 500 million entities per split. For prod, this will
		// result in ~500 splits, which roughly matches the maximum number of
		// workers we can have.
		// The runner can decide to split further if some splits finishes earlier
		// than the others.
		InitialSplitSize: 500_000_000,
	}

	namespaces := dsutils.GetAllNamespaces(s, *cloudProject)
	logStreamKeys := dsutils.GetAllKeysWithHexPrefix(s, *cloudProject, namespaces, "LogStream", readOpts)
	logstream.BackfillExpireAtFromCreated(s, *cloudProject, logStreamKeys, backfillOpts)

	return beamx.Run(ctx, p)
}
