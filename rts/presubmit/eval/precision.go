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
	"path/filepath"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"golang.org/x/sync/errgroup"
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

func (r *evalRun) evaluatePrecision(ctx context.Context) (*Precision, error) {
	eg, ctx := errgroup.WithContext(ctx)

	// Fetch test durations.
	durationC := make(chan *TestDuration)
	eg.Go(func() error {
		defer close(durationC)

		durations, err := r.testDurations(ctx)
		if err != nil {
			return err
		}

		for _, d := range durations {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case durationC <- d:
			}
		}
		return ctx.Err()
	})

	// Run the algorithm in r.Concurrency goroutines.
	var total int64
	var forecast int64
	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			for d := range durationC {
				atomic.AddInt64(&total, int64(d.Duration))

				changedFiles, err := r.gerrit.ChangedFiles(ctx, &d.Patchset)
				if err != nil {
					return err
				}

				out, err := r.Algorithm(ctx, Input{
					ChangedFiles: changedFiles,
					Test:         &d.Test,
				})
				if err != nil {
					return err
				}
				if out.ShouldRun {
					atomic.AddInt64(&forecast, int64(d.Duration))
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

// testDurations returns a sample of test durations, provided by the backend.
// Might cache results.
func (r *evalRun) testDurations(ctx context.Context) ([]*TestDuration, error) {
	now := clock.Now(ctx)

	cacheFile := cacheFile(filepath.Join(r.CacheDir, r.Backend.Name(), "test-durations"))
	var cacheContent struct {
		Res *TestDurationsSampleResponse `json:"res"`
		Exp time.Time                    `json:"exp"`
	}
	if cacheFile.TryRead(ctx, &cacheContent) && cacheContent.Exp.After(now) {
		return cacheContent.Res.TestDurations, nil
	}

	logging.Infof(ctx, "Fetching a recent sample of test durations. It may take a couple of minutes...")
	res, err := r.Backend.TestDurationsSample(TestDurationsSampleRequest{
		Context:       ctx,
		Authenticator: r.auth,
	})
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "Fetched %d test durations", len(res.TestDurations))

	if res.TTL > 0 {
		cacheContent.Res = res
		cacheContent.Exp = now.Add(res.TTL)
		cacheFile.TryWrite(ctx, cacheContent)
	}
	return res.TestDurations, nil
}
