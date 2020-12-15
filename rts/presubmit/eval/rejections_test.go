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
	"strconv"
	"testing"

	evalpb "go.chromium.org/luci/rts/presubmit/eval/proto"
	"golang.org/x/sync/errgroup"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnalyzeRejections(t *testing.T) {
	t.Parallel()
	Convey(`AnalyzeRejections`, t, func() {
		ctx := context.Background()
		rejections := []*evalpb.Rejection{
			{
				FailedTestVariants: []*evalpb.TestVariant{
					{Id: "1"},
					{Id: "2"},
				},
			},
			{
				FailedTestVariants: []*evalpb.TestVariant{
					{Id: "3"},
				},
			},
			{
				FailedTestVariants: []*evalpb.TestVariant{
					{Id: "4"},
					{Id: "1"},
				},
			},
		}

		strategy := func(ctx context.Context, in Input, out *Output) error {
			for i, tv := range in.TestVariants {
				rank, err := strconv.Atoi(tv.Id)
				if err != nil {
					return err
				}
				out.TestVariantAffectedness[i].Distance = float64(rank)
				out.TestVariantAffectedness[i].Rank = rank
			}
			return nil
		}

		eg, ctx := errgroup.WithContext(ctx)

		rejC := make(chan *evalpb.Rejection)
		var actual []analyzedRejection
		eg.Go(func() error {
			return analyzeRejections(ctx, rejC, strategy, func(ctx context.Context, rej analyzedRejection) error {
				actual = append(actual, rej)
				return nil
			})
		})

		eg.Go(func() error {
			defer close(rejC)
			for _, r := range rejections {
				rejC <- r
			}
			return nil
		})

		So(eg.Wait(), ShouldBeNil)
		So(actual, ShouldHaveLength, 3)

		So(actual[0].Rejection, ShouldResemble, rejections[0])
		So(actual[0].Affectedness, ShouldResemble, AffectednessSlice{
			{Distance: 1, Rank: 1},
			{Distance: 2, Rank: 2},
		})
		So(actual[0].Closest, ShouldResemble, Affectedness{Distance: 1, Rank: 1})

		So(actual[1].Rejection, ShouldResemble, rejections[1])
		So(actual[1].Affectedness, ShouldResemble, AffectednessSlice{{Distance: 3, Rank: 3}})
		So(actual[1].Closest, ShouldResemble, Affectedness{Distance: 3, Rank: 3})

		So(actual[2].Rejection, ShouldResemble, rejections[2])
		So(actual[2].Affectedness, ShouldResemble, AffectednessSlice{
			{Distance: 4, Rank: 4},
			{Distance: 1, Rank: 1},
		})
		So(actual[2].Closest, ShouldResemble, Affectedness{Distance: 1, Rank: 1})
	})
}
