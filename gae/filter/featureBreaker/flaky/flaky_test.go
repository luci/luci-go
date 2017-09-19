// Copyright 2017 The LUCI Authors.
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

package flaky

import (
	"testing"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/service/datastore"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFlakyErrors(t *testing.T) {
	t.Parallel()

	sample := func(cb featureBreaker.BreakFeatureCallback, feature string) map[error]int {
		out := map[error]int{}
		for i := 0; i < 1000; i++ {
			out[cb(context.Background(), feature)]++
		}
		return out
	}

	Convey("Deadlines only", t, func() {
		params := Params{
			DeadlineProbability:              0.05,
			ConcurrentTransactionProbability: 0.1,
		}
		So(sample(Errors(params), "AllocateIDs"), ShouldResemble, map[error]int{
			nil:                 950,
			ErrFlakyRPCDeadline: 50,
		})
	})

	Convey("Deadlines and commits", t, func() {
		params := Params{
			DeadlineProbability:              0.05,
			ConcurrentTransactionProbability: 0.1,
		}
		So(sample(Errors(params), "CommitTransaction"), ShouldResemble, map[error]int{
			nil: 853,
			datastore.ErrConcurrentTransaction: 95,
			ErrFlakyRPCDeadline:                52,
		})
	})
}
