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

package resultcollector

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"google.golang.org/protobuf/proto"

	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/analyzedtestvariants"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
)

const (
	taskClass                 = "result-collection"
	queue                     = "result-collection"
	maxBatchSize              = 500
	maxConcurrentBatchRequest = 10
)

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass() {
	tq.RegisterTaskClass(tq.TaskClass{
		ID:        taskClass,
		Prototype: &taskspb.CollectTestResults{},
		Queue:     queue,
		Kind:      tq.NonTransactional,
		Handler: func(ctx context.Context, payload proto.Message) error {
			task := payload.(*taskspb.CollectTestResults)
			return collectTestResults(ctx, task)
		},
	})
}

// Schedule enqueues a task to get test results of interesting test variants
// from an invocation.
//
// Interesting test variants are the analyzed test variants with any unexpected
// results.
func Schedule(ctx context.Context, inv *rdbpb.Invocation, rdbHost, builder string, isPreSubmit, contributedToCLSubmission bool) error {
	return tq.AddTask(ctx, &tq.Task{
		Title: fmt.Sprintf("%s", inv.Name),
		Payload: &taskspb.CollectTestResults{
			Resultdb: &taskspb.ResultDB{
				Invocation: inv,
				Host:       rdbHost,
			},
			Builder:                   builder,
			IsPreSubmit:               isPreSubmit,
			ContributedToClSubmission: contributedToCLSubmission,
		},
	})
}

func collectTestResults(ctx context.Context, task *taskspb.CollectTestResults) error {
	client, err := resultdb.NewClient(ctx, task.Resultdb.Host)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)
	batchC := make(chan []*rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier)

	eg.Go(func() error {
		return batchSaveVerdicts(ctx, task, client, batchC)
	})

	eg.Go(func() error {
		defer close(batchC)
		return queryInterestingTestVariants(ctx, task.Resultdb.Invocation.Realm, task.Builder, batchC)
	})

	return eg.Wait()
}

// queryInterestingTestVariants queries analyzed test variants with any
// unexpected results.
func queryInterestingTestVariants(ctx context.Context, realm, builder string, batchC chan []*rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier) error {
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	tvis := make([]*rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier, 0, maxBatchSize)
	f := func(tv *atvpb.AnalyzedTestVariant) error {
		tvis = append(tvis, &rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier{
			TestId:      tv.TestId,
			VariantHash: tv.VariantHash,
		})

		if len(tvis) >= maxBatchSize {
			// Handle a full batch.
			select {
			case <-ctx.Done():
				return ctx.Err()
			case batchC <- tvis:
			}
			tvis = make([]*rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier, 0, maxBatchSize)
		}
		return nil
	}

	err := analyzedtestvariants.QueryTestVariantsByBuilder(ctx, realm, builder, f)
	if err != nil {
		return err
	}

	if len(tvis) > 0 {
		// Handle the last batch.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case batchC <- tvis:
		}
	}
	return nil
}

// batchSaveVerdicts batch get test variants from a build invocation and save
// the results of those test variants in Verdicts.
func batchSaveVerdicts(ctx context.Context, task *taskspb.CollectTestResults, client *resultdb.Client, batchC chan []*rdbpb.BatchGetTestVariantsRequest_TestVariantIdentifier) error {
	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Limit the number of concurrent batch requests.
	sem := semaphore.NewWeighted(maxConcurrentBatchRequest)

	for tvis := range batchC {
		// See https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		tvis := tvis
		eg.Go(func() error {
			// Limit concurrent batch requests.
			if err := sem.Acquire(ctx, 1); err != nil {
				return err
			}
			defer sem.Release(1)

			tvs, err := client.BatchGetTestVariants(ctx, &rdbpb.BatchGetTestVariantsRequest{
				Invocation:   task.Resultdb.Invocation.Name,
				TestVariants: tvis,
			})
			if err != nil {
				return err
			}

			return createVerdicts(ctx, task, tvs)
		})
	}

	return eg.Wait()
}
