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

package eval

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Precision is result of algorithm precision evaluation.
// A precise algorithm selects only affected tests.
type Precision struct {
	// SampleDuration is the sum of test durations in the analyzed sample of test
	// results.
	SampleDuration time.Duration

	// ForecastDuration is the sum of test durations for tests selected by the RTS
	// algorithm. It is value between 0 and SampleDuration.
	// The lower the number the better.
	ForecastDuration time.Duration
}

func (r *evalRun) evaluatePrecision(ctx context.Context, durationC <-chan *evalpb.TestDuration) (*Precision, error) {
	eg, ctx := errgroup.WithContext(ctx)

	// Run the algorithm in r.Concurrency goroutines.
	var total, forecast int64 // in nanoseconds
	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			for td := range durationC {
				durNano := int64(td.Duration.AsDuration())
				atomic.AddInt64(&total, durNano)

				changedFiles, err := r.changedFiles(ctx, td.Patchsets...)
				if err != nil {
					return err
				}

				out, err := r.Algorithm(ctx, Input{
					ChangedFiles: changedFiles,
					Test:         td.Test,
				})
				if err != nil {
					return err
				}
				if out.ShouldRun {
					atomic.AddInt64(&forecast, durNano)
				}
			}
			return ctx.Err()
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return &Precision{
		SampleDuration:   time.Duration(total),
		ForecastDuration: time.Duration(forecast),
	}, nil
}
