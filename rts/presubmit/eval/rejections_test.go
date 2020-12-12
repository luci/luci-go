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
	"strconv"
	"strings"
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

func TestPrintLostRejection(t *testing.T) {
	t.Parallel()

	assert := func(rej *evalpb.Rejection, expectedText string) {
		buf := &bytes.Buffer{}
		p := rejectionPrinter{printer: newPrinter(buf)}
		So(p.rejection(rej), ShouldBeNil)
		expectedText = strings.Replace(expectedText, "\t", "  ", -1)
		So(buf.String(), ShouldEqual, expectedText)
	}

	ps1 := &evalpb.GerritPatchset{
		Change: &evalpb.GerritChange{
			Host:    "chromium-review.googlesource.com",
			Project: "chromium/src",
			Number:  123,
		},
		Patchset: 4,
	}
	ps2 := &evalpb.GerritPatchset{
		Change: &evalpb.GerritChange{
			Host:    "chromium-review.googlesource.com",
			Project: "chromium/src",
			Number:  223,
		},
		Patchset: 4,
	}

	Convey(`PrintLostRejection`, t, func() {
		Convey(`Basic`, func() {
			rej := &evalpb.Rejection{
				Patchsets:          []*evalpb.GerritPatchset{ps1},
				FailedTestVariants: []*evalpb.TestVariant{{Id: "test1"}},
			}

			assert(rej, `Lost rejection:
	https://chromium-review.googlesource.com/c/123/4
	Failed and not selected tests:
		- <empty test variant>
			- test1
`)
		})

		Convey(`With file name`, func() {
			rej := &evalpb.Rejection{
				Patchsets: []*evalpb.GerritPatchset{ps1},
				FailedTestVariants: []*evalpb.TestVariant{{
					Id:       "test1",
					FileName: "test.cc",
				}},
			}

			assert(rej, `Lost rejection:
	https://chromium-review.googlesource.com/c/123/4
	Failed and not selected tests:
		- <empty test variant>
			- test1
				in test.cc
`)
		})

		Convey(`Multiple variants`, func() {
			rej := &evalpb.Rejection{
				Patchsets: []*evalpb.GerritPatchset{ps1},
				FailedTestVariants: []*evalpb.TestVariant{
					{
						Id: "test1",
						Variant: map[string]string{
							"a": "0",
						},
					},
					{
						Id: "test2",
						Variant: map[string]string{
							"a": "0",
						},
					},
					{
						Id: "test1",
						Variant: map[string]string{
							"a": "0",
							"b": "0",
						},
					},
				},
			}

			assert(rej, `Lost rejection:
	https://chromium-review.googlesource.com/c/123/4
	Failed and not selected tests:
		- a:0
			- test1
			- test2
		- a:0 | b:0
			- test1
`)
		})

		Convey(`Two patchsets`, func() {
			rej := &evalpb.Rejection{
				Patchsets:          []*evalpb.GerritPatchset{ps1, ps2},
				FailedTestVariants: []*evalpb.TestVariant{{Id: "test1"}},
			}

			assert(rej, `Lost rejection:
	- patchsets:
		https://chromium-review.googlesource.com/c/123/4
		https://chromium-review.googlesource.com/c/223/4
	Failed and not selected tests:
		- <empty test variant>
			- test1
`)
		})
	})
}
