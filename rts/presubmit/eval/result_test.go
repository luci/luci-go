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
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResultPrint(t *testing.T) {
	t.Parallel()

	Convey(`Print`, t, func() {
		r := Result{
			TotalRejections:   100,
			TotalTestFailures: 100,
			TotalDuration:     time.Hour,
		}
		r.Thresholds.init(AffectednessSlice{{Distance: 1}, {Distance: 20}, {Distance: 30}, {Distance: 40}})
		r.Thresholds[0][0].SavedDuration = time.Hour
		r.Thresholds[98][98].PreservedRejections = 99
		r.Thresholds[98][98].PreservedTestFailures = 99
		r.Thresholds[98][98].SavedDuration = 15 * time.Minute
		r.Thresholds[99][99].PreservedTestFailures = 100
		r.Thresholds[99][99].PreservedRejections = 100
		r.Thresholds[99][99].SavedDuration = 30 * time.Minute
		buf := &bytes.Buffer{}
		r.Print(buf, 0)
		So(buf.String(), ShouldEqual, `
ChangeRecall | Savings | TestRecall | Distance, ChangeRecall | Rank,     ChangeRecall
-------------------------------------------------------------------------------------
  0.00%      | 100.00% |   0.00%    |  1.000      1.00%      | 0           1.00%
 99.00%      |  25.00% |  99.00%    | 40.000     99.00%      | 0          99.00%
100.00%      |  50.00% | 100.00%    | 40.000    100.00%      | 0         100.00%

based on 100 rejections, 100 test failures, 1h0m0s testing time
`[1:])
	})
}
