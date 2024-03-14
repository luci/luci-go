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
		64,
		"The number of go workers in a backfill beam worker.",
	)

	// entityShards controls the number of shards (per namespace) the datastore
	// keys are divided into. This affects the maximum parallelism level of jobs
	// in later stages. Should be roughly an order of magnitude larger than the
	// maximum number of beam workers so that
	// 1. each beam worker gets enough entities so datastore operations can be
	// batched, and
	// 2. beam workers finished the assigned shard early can pick another shard to
	// work on.
	entityShards = flag.Int("entity-shards", 8192, "The number of shards per namespace per datastore entity type when outputting entities.")
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
		BatchSize: 512,
		Workers:   *backfillWorkers,
		// Entities created this year all have the expire at property populated.
		// Only processing old entities also let us commit updates safely without
		// using transaction.
		SkipCreatedAfter: time.Date(2024, time.January, 0, 0, 0, 0, 0, time.UTC),
		Expiry:           coordinator.LogStreamExpiry,
	}

	readOpts := dsutils.ReadOptions{
		ParentKind:   "LogStream",
		OutputShards: *entityShards,
		// There are 64, but 10 should be more than enough.
		// Can split to at most 16^10 + 1 splits.
		HexPrefixLength: 10,
	}

	namespaces := dsutils.GetAllNamespaces(s, *cloudProject)
	logStreamKeys := dsutils.GetAllKeysWithHexPrefix(s, *cloudProject, namespaces, "LogStream", readOpts)
	logstream.BackfillExpireAtFromCreated(s, *cloudProject, logStreamKeys, backfillOpts)
	logStreamStateKeys := dsutils.GetAllKeysWithHexPrefix(s, *cloudProject, namespaces, "LogStreamState", readOpts)
	logstream.BackfillExpireAtFromCreated(s, *cloudProject, logStreamStateKeys, backfillOpts)

	return beamx.Run(ctx, p)
}
