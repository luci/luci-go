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
			Thresholds: []*Threshold{
				{
					Savings: 1,
				},
				{
					Value:                Affectedness{Distance: 10},
					DistanceChangeRecall: 0.99,
					RankChangeRecall:     0.99,
					ChangeRecall:         0.99,
					TestRecall:           0.99,
					Savings:              0.25,
				},
				{
					Value:                Affectedness{Distance: 40},
					DistanceChangeRecall: 1,
					RankChangeRecall:     1,
					ChangeRecall:         1,
					TestRecall:           1,
					Savings:              0.5,
				},
			},
		}

		buf := &bytes.Buffer{}
		r.Print(buf, 0)
		So(buf.String(), ShouldEqual, `
ChangeRecall | Savings | TestRecall | Distance, ChangeRecall | Rank, ChangeRecall
---------------------------------------------------------------------------------
  0.00%      | 100.00% |   0.00%    |  0.000              0% | 0               0%
 99.00%      |  25.00% |  99.00%    | 10.000             99% | 0              99%
100.00%      |  50.00% | 100.00%    | 40.000            100% | 0             100%

based on 100 rejections, 100 test failures, 1h0m0s testing time
`[1:])
	})
}
