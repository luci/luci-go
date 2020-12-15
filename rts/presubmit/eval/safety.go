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
	"bytes"
	"context"
	"math"
	"sort"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/logging"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Safety is how safe the candidate strategy is.
// A safe strategy does not let bad CLs pass CQ.
type Safety struct {
	ChangeRecall ChangeRecall
	TestRecall   TestRecall
}

// ChangeRecall represents the fraction of code change rejections
// that were preserved by the candidate strategy.
type ChangeRecall struct {
	// TotalRejections is the total number of analyzed rejections.
	TotalRejections int

	// LostRejections are the rejections that would not be preserved
	// by the candidate strategy, i.e. the bad patchsets would land.
	// The strategy did not select any of the failed tests in these rejections.
	//
	// Ideally this slice is empty.
	LostRejections []*evalpb.Rejection
}

func (r *ChangeRecall) preserved() int {
	return r.TotalRejections - len(r.LostRejections)
}

// Score returns the fraction of rejections that were preserved.
// May return NaN.
func (r *ChangeRecall) Score() float64 {
	if r.TotalRejections == 0 {
		return math.NaN()
	}
	return float64(r.preserved()) / float64(r.TotalRejections)
}

// TestRecall represents the fraction of test failures
// that were preserved by the candidate strategy.
// If a test is not selected by the strategy, then its failure is not
// preserved.
type TestRecall struct {
	// TotalFailures is the total number of analyzed test failures.
	TotalFailures int

	// LostFailures are the failures which the candidate strategy did not preserve.
	LostFailures []LostTestFailure
}

func (r *TestRecall) preserved() int {
	return r.TotalFailures - len(r.LostFailures)
}

// Score returns the fraction of test failures that were preserved.
// May return NaN.
func (r *TestRecall) Score() float64 {
	if r.TotalFailures == 0 {
		return math.NaN()
	}
	return float64(r.preserved()) / float64(r.TotalFailures)
}

// LostTestFailure is a failure of a test that the candidate strategy did not select.
type LostTestFailure struct {
	Rejection   *evalpb.Rejection
	TestVariant *evalpb.TestVariant
}

// evaluateSafety reads rejections from r.rejectionC,
// updates r.res.Safety and calls r.maybeReportProgress.
func (r *evalRun) evaluateSafety(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			return r.analyzeRejections(ctx)
		})
	}
	return eg.Wait()
}

func (r *evalRun) analyzeRejections(ctx context.Context) error {
	cr := &r.res.Safety.ChangeRecall
	tr := &r.res.Safety.TestRecall

	buf := &bytes.Buffer{}
	p := newPrinter(buf)
	return analyzeRejections(ctx, r.History.RejectionC, r.Strategy, func(ctx context.Context, rej analyzedRejection) error {
		// The rejection is preserved if at least one test ran.
		// Conclude that based on the closest test.
		preservedRejection := r.shouldRun(rej.Closest)

		if !preservedRejection && r.LogLostRejections {
			buf.Reset()
			printLostRejection(p, rej.Rejection)
			logging.Infof(ctx, "%s", buf.Bytes())
		}

		r.mu.Lock()
		defer r.mu.Unlock()
		cr.TotalRejections++
		tr.TotalFailures += len(rej.FailedTestVariants)
		if !preservedRejection {
			cr.LostRejections = append(cr.LostRejections, rej.Rejection)
		}
		for i, af := range rej.Affectedness {
			if !r.shouldRun(af) {
				tr.LostFailures = append(tr.LostFailures, LostTestFailure{
					Rejection:   rej.Rejection,
					TestVariant: rej.FailedTestVariants[i],
				})
			}
		}

		r.maybeReportProgress(ctx)
		return nil
	})
}

// printLostRejection prints a rejection that wouldn't be preserved by the
// candidate strategy.
func printLostRejection(p *printer, rej *evalpb.Rejection) error {
	pf := p.printf

	pf("Lost rejection:\n")
	p.Level++

	// Print patchsets.
	if len(rej.Patchsets) == 1 {
		printLostPatchset(p, rej.Patchsets[0])
	} else {
		pf("- patchsets:\n")
		p.Level++
		for _, ps := range rej.Patchsets {
			printLostPatchset(p, ps)
		}
		p.Level--
	}

	printTestVariants(p, rej.FailedTestVariants)

	p.Level--
	return p.err
}

func printLostPatchset(p *printer, ps *evalpb.GerritPatchset) {
	p.printf("%s\n", psURL(ps))

	paths := make([]string, len(ps.ChangedFiles))
	for i, f := range ps.ChangedFiles {
		paths[i] = f.Path
	}
	sort.Strings(paths)

	p.Level++
	for _, f := range paths {
		p.printf("%s\n", f)
	}
	p.Level--
}

// printTestVariants prints tests grouped by variant.
func printTestVariants(p *printer, testVariants []*evalpb.TestVariant) {
	pf := p.printf

	// Group by variant.
	byVariant := map[string][]*evalpb.TestVariant{}
	var keys []string
	for _, tv := range testVariants {
		key := variantString(tv.Variant)
		tests, ok := byVariant[key]
		byVariant[key] = append(tests, tv)
		if !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	// Print tests grouped by variant.
	pf("Failed and not selected tests:\n")
	p.Level++
	for _, key := range keys {
		pf("- ")
		if key == "" {
			pf("<empty test variant>\n")
		} else {
			pf("%s\n", key)
		}

		ts := byVariant[key]
		sort.Slice(ts, func(i, j int) bool {
			return ts[i].Id < ts[j].Id
		})

		p.Level++
		for _, t := range ts {
			p.printf("- %s\n", t.Id)
			if t.FileName != "" {
				p.printf("  in %s\n", t.FileName)
			}
		}
		p.Level--
	}
	p.Level--
}
