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
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Safety is result of algorithm safety evaluation.
// A safe algorithm does not let bad CLs pass CQ.
type Safety struct {
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

func (r *evalRun) evaluateSafety(ctx context.Context, rejectionC chan *evalpb.Rejection) (*Safety, error) {
	eg, ctx := errgroup.WithContext(ctx)

	var ret Safety
	var mu sync.Mutex
	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			buf := &bytes.Buffer{}
			p := newPrinter(buf)
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()

				case rej, ok := <-rejectionC:
					if !ok {
						return nil
					}

					eligible, wouldReject, err := r.processRejection(ctx, rej)

					mu.Lock()
					ret.TotalRejections++
					if err == nil && eligible {
						ret.EligibleRejections++
						if !wouldReject {
							ret.LostRejections = append(ret.LostRejections, rej)
						}
					}
					mu.Unlock()
					if err != nil {
						return errors.Annotate(err, "failed to process rejection %q", rej).Err()
					}

					if eligible && !wouldReject {
						buf.Reset()
						printLostRejection(p, rej)
						logging.Infof(ctx, "%s", buf.Bytes())
					}
				}
			}
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return &ret, nil
}

func (r *evalRun) processRejection(ctx context.Context, rej *evalpb.Rejection) (eligible, wouldReject bool, err error) {
	// TODO(crbug.com/1112125): add support for CL stacks.
	// This call returns only files modified in the particular patchset and
	// ignores possible parent CLs that were also tested.

	// TODO(crbug.com/1112125): skip the patchset if it has a ton of failed tests.
	// Most RTS algorithms would reject such a patchset, so it represents noise.

	files, err := r.changedFiles(ctx, rej.Patchsets...)
	switch {
	case len(files) == 0:
		// The CL is deleted  => not eligible.
		err = nil
		return

	case err != nil:
		err = errors.Annotate(err, "failed to read changed files of %s", rej).Err()
		return
	}

	// Compare the prediction to facts.
	in := Input{ChangedFiles: files}
	for _, tv := range rej.FailedTestVariants {
		in.TestVariant = tv
		var out Output
		if out, err = r.Algorithm(ctx, in); err != nil {
			err = errors.Annotate(err, "RTS algorithm failed").Err()
			return
		}
		eligible = true

		if out.ShouldRun {
			// At least one failed test would run => the bad patchset would be rejected.
			wouldReject = true
			return
		}
	}
	return
}

// changedFiles retrieves changed files of all patchsets in the rejection.
func (r *evalRun) changedFiles(ctx context.Context, patchsets ...*evalpb.GerritPatchset) ([]*SourceFile, error) {
	var ret []*SourceFile
	var mu sync.Mutex
	err := parallel.FanOutIn(func(workC chan<- func() error) {
		for _, ps := range patchsets {
			ps := ps
			workC <- func() error {
				changedFiles, err := r.gerrit.ChangedFiles(ctx, ps)
				if err != nil {
					return err
				}

				repo := fmt.Sprintf("https://%s/%s", ps.Change.Host, strings.TrimSuffix(ps.Change.Project, ".git"))

				mu.Lock()
				defer mu.Unlock()
				for _, path := range changedFiles {
					ret = append(ret, &SourceFile{
						Repo: repo,
						Path: "//" + path,
					})
				}
				return nil
			}
		}
	})
	if err != nil {
		return nil, err
	}
	return ret, err
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
