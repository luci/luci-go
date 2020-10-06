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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
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

	// PreservedRejections is the number of rejections that would be preserved
	// by the candidate algorithm. Ideally this number equals EligibleRejections.
	PreservedRejections int
}

func (r *evalRun) evaluateSafety(ctx context.Context, rejectionC chan *evalpb.Rejection) (*Safety, error) {
	eg, ctx := errgroup.WithContext(ctx)

	var totalRejections, eligibleCount, preservedRejections int32
	for i := 0; i < r.Concurrency; i++ {
		eg.Go(func() error {
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()

				case rej, ok := <-rejectionC:
					if !ok {
						return nil
					}

					atomic.AddInt32(&totalRejections, 1)

					switch eligible, wouldReject, err := r.processRejection(ctx, rej); {
					case err != nil:
						return errors.Annotate(err, "failed to process rejection %s", rej).Err()
					case eligible:
						atomic.AddInt32(&eligibleCount, 1)
						if wouldReject {
							atomic.AddInt32(&preservedRejections, 1)
						}
					}
				}
			}
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return &Safety{
		TotalRejections:     int(totalRejections),
		EligibleRejections:  int(eligibleCount),
		PreservedRejections: int(preservedRejections),
	}, nil
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
