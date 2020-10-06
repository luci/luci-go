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
	Safety       Safety
	Efficiency   Efficiency
	TotalRecords int
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

	// Analyze efficiency.
	durationC := make(chan *evalpb.TestDuration)
	eg.Go(func() error {
		efficiency, err := r.evaluateEfficiency(ctx, durationC)
		if err != nil {
			return errors.Annotate(err, "failed to evaluate efficiency").Err()
		}
		ret.Efficiency = *efficiency
		return nil
	})

	// Play back the history.
	eg.Go(func() error {
		defer func() {
			close(rejectionC)
			close(durationC)
			r.History.Close()
		}()

		for {
			rec, err := r.History.Read()
			switch {
			case err == io.EOF:
				return nil
			case err != nil:
				return errors.Annotate(err, "failed to read history").Err()
			}

			ret.TotalRecords++

			// Send the record to the appropriate channel.
			switch data := rec.Data.(type) {
			case *evalpb.Record_Rejection:
				select {
				case <-ctx.Done():
					return ctx.Err()
				case rejectionC <- data.Rejection:
				}
			case *evalpb.Record_TestDuration:
				select {
				case <-ctx.Done():
					return ctx.Err()
				case durationC <- data.TestDuration:
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
	p := func(format string, args ...interface{}) {
		if err == nil {
			_, err = fmt.Fprintf(w, format, args...)
		}
	}

	p("Safety:\n")
	switch {
	case r.Safety.TotalRejections == 0:
		p("  Evaluation failed: rejections not found\n")

	case r.Safety.EligibleRejections == 0:
		p("  Evaluation failed: all %d rejections are ineligible.\n", r.Safety.TotalRejections)

	default:
		p("  Score: %.0f%%\n", float64(100*r.Safety.PreservedRejections)/float64(r.Safety.EligibleRejections))
		p("  # of eligible rejections: %d\n", r.Safety.EligibleRejections)
		p("  # of them preserved by this RTS: %d\n", r.Safety.PreservedRejections)
	}

	p("Efficiency:\n")
	if r.Efficiency.SampleDuration == 0 {
		p("  Evaluation failed: no test results with duration\n")
	} else {
		saved := r.Efficiency.SampleDuration - r.Efficiency.ForecastDuration
		p("  Saved: %.0f%%\n", float64(100*saved)/float64(r.Efficiency.SampleDuration))
		p("  Compute time in the sample: %s\n", r.Efficiency.SampleDuration)
		p("  Forecasted compute time: %s\n", r.Efficiency.ForecastDuration)
	}

	p("Total records: %d\n", r.TotalRecords)
	return
}
