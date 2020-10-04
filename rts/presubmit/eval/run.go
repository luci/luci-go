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
	"io"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
)

// Result is the result of evaluation.
type Result struct {
	Safety    Safety
	Precision Precision
}

type evalRun struct {
	Eval

	auth   *auth.Authenticator
	gerrit *gerritClient
}

func (r *evalRun) run(ctx context.Context) (*Result, error) {
	if err := r.Init(ctx); err != nil {
		return nil, err
	}

	ret := &Result{}

	eg, ctx := errgroup.WithContext(ctx)
	defer eg.Wait()

	// Analyze safety.
	rejectionC := make(chan *evalpb.Rejection)
	eg.Go(func() error {
		safety, err := r.evaluateSafety(ctx, rejectionC)
		if err != nil {
			return errors.Annotate(err, "failed to evaluate safety").Err()
		}
		ret.Safety = *safety
		return nil
	})

	// TODO(nodir): analyze precision.

	// Play back the history.
	eg.Go(func() error {
		defer close(rejectionC)
		defer r.History.Close()
		for {
			rec, err := r.History.Read()
			switch {
			case err == io.EOF:
				return nil
			case err != nil:
				return errors.Annotate(err, "failed to read history").Err()
			}
			switch data := rec.Data.(type) {
			case *evalpb.Record_Rejection:
				select {
				case <-ctx.Done():
					return ctx.Err()
				case rejectionC <- data.Rejection:
				}
			default:
				panic(fmt.Sprintf("unexpected record %s", rec))
			}
		}
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}
	return ret, nil
}

// Print prints the results to w.
func (r *Result) Print(w io.Writer) (err error) {
	switch {
	case r.Safety.TotalRejections == 0:
		_, err = fmt.Printf("Evaluation failed: rejections not found\n")

	case r.Safety.EligibleRejections == 0:
		_, err = fmt.Printf("Evaluation failed: all %d rejections are ineligible.\n", r.Safety.TotalRejections)

	default:
		fmt.Printf("Total analyzed rejections: %d\n", r.Safety.TotalRejections)
		_, err = fmt.Printf("Safety score: %.2f (%d/%d)\n",
			float64(r.Safety.PreservedRejections)/float64(r.Safety.EligibleRejections),
			r.Safety.PreservedRejections,
			r.Safety.EligibleRejections,
		)
	}

	// TODO(crbug.com/1112125): print precision.
	return
}
