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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Safety is result of algorithm safety evaluation.
// A safe algorithm does not let bad CLs pass CQ.
type Safety struct {
	ChangeRecall ChangeRecall
	TestRecall   TestRecall
}

// ChangeRecall represents the fraction of eligible code change rejections
// that were preserved by the RTS algorithm.
type ChangeRecall struct {
	// TotalRejections is the total number of analyzed rejections.
	TotalRejections int

	// EligibleRejections is the number of rejections eligible for safety
	// evaluation.
	EligibleRejections int

	// LostRejections are the rejections that would not be preserved
	// by the candidate algorithm, i.e. the bad patchsets would land.
	// The candidate RTS algorithm did not select any of the failed tests
	// in these rejections.
	//
	// Ideally this slice is empty.
	LostRejections []*evalpb.Rejection
}

func (r *ChangeRecall) preserved() int {
	return r.EligibleRejections - len(r.LostRejections)
}

// Score returns the fraction of eligible rejections that were preserved.
// May return NaN.
func (r *ChangeRecall) Score() float64 {
	if r.EligibleRejections == 0 {
		return math.NaN()
	}
	return float64(r.preserved()) / float64(r.EligibleRejections)
}

// TestRecall represents the fraction of eligible test failures
// that were preserved by the RTS algorithm.
// If a test is not selected by the algorithm, then its failure is not
// preserved.
type TestRecall struct {
	// TotalFailures is the total number of analyzed test failures.
	TotalFailures int

	// EligibleFailures is the number of failures eligible for safety
	// evaluation.
	EligibleFailures int

	// LostFailures are the failures which the RTS algorithm did not preserve.
	LostFailures []LostTestFailure
}

func (r *TestRecall) preserved() int {
	return r.EligibleFailures - len(r.LostFailures)
}

// Score returns the fraction of eligible test failures that were preserved.
// May return NaN.
func (r *TestRecall) Score() float64 {
	if r.EligibleFailures == 0 {
		return math.NaN()
	}
	return float64(r.preserved()) / float64(r.EligibleFailures)
}

// LostTestFailure is a failure of a test that the RTS algorithm did not select.
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
			se := safetyEval{evalRun: r}
			return se.eval(ctx)
		})
	}

	return eg.Wait()
}

type safetyEval struct {
	*evalRun
	out Output
}

func (e *safetyEval) eval(ctx context.Context) error {
	cr := &e.res.Safety.ChangeRecall
	tr := &e.res.Safety.TestRecall

	buf := &bytes.Buffer{}
	p := newPrinter(buf)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case rej, ok := <-e.rejectionC:
			if !ok {
				return nil
			}

			eligible, err := e.processRejection(ctx, rej)
			if err != nil {
				return errors.Annotate(err, "failed to process rejection %q", rej).Err()
			}

			// All tests were skipped => the rejection is lost.
			lostRejection := true
			for _, dist := range e.out.TestVariantDistances {
				if dist <= e.MaxDistance {
					lostRejection = false
					break
				}
			}

			e.mu.Lock()
			cr.TotalRejections++
			tr.TotalFailures += len(rej.FailedTestVariants)
			if eligible {
				cr.EligibleRejections++
				tr.EligibleFailures += len(rej.FailedTestVariants)
				if lostRejection {
					cr.LostRejections = append(cr.LostRejections, rej)
				}
				for i, dist := range e.out.TestVariantDistances {
					if dist > e.MaxDistance {
						tr.LostFailures = append(tr.LostFailures, LostTestFailure{
							Rejection: rej,
							// Note: this assumes that in.TestVariants == rej.FailedTestVariants
							TestVariant: rej.FailedTestVariants[i],
						})
					}
				}
			}

			e.maybeReportProgress(ctx)
			e.mu.Unlock()

			if eligible && lostRejection && e.LogLostRejections {
				buf.Reset()
				printLostRejection(p, rej)
				logging.Infof(ctx, "%s", buf.Bytes())
			}
		}
	}
}

func (e *safetyEval) processRejection(ctx context.Context, rej *evalpb.Rejection) (eligible bool, err error) {
	// TODO(crbug.com/1112125): add support for CL stacks.
	// This call returns only files modified in the particular patchset and
	// ignores possible parent CLs that were also tested.

	// TODO(crbug.com/1112125): skip the patchset if it has a ton of failed tests.
	// Most RTS algorithms would reject such a patchset, so it represents noise.

	// Select tests.
	in := Input{
		TestVariants: rej.FailedTestVariants,
	}
	in.ensureChangedFilesInclude(rej.Patchsets...)
	if cap(e.out.TestVariantDistances) < len(in.TestVariants) {
		e.out.TestVariantDistances = make([]float64, len(in.TestVariants))
	} else {
		e.out.TestVariantDistances = e.out.TestVariantDistances[:len(in.TestVariants)]
		for i := range e.out.TestVariantDistances {
			e.out.TestVariantDistances[i] = 0
		}
	}
	if err := e.Algorithm(ctx, in, &e.out); err != nil {
		return false, errors.Annotate(err, "RTS algorithm failed").Err()
	}
	return true, nil
}

// printLostRejection prints a rejection that wouldn't be preserved by the
// candidate RTS algorithm.
func printLostRejection(p *printer, rej *evalpb.Rejection) error {
	pf := p.printf

	pf("Lost rejection:\n")
	p.Level++

	// Print patchsets.
	if len(rej.Patchsets) == 1 {
		pf("%s\n", psURL(rej.Patchsets[0]))
	} else {
		pf("- patchsets:\n")
		p.Level++
		for _, ps := range rej.Patchsets {
			pf("%s\n", psURL(ps))
		}
		p.Level--
	}

	printTestVariants(p, rej.FailedTestVariants)

	p.Level--
	return p.err
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
