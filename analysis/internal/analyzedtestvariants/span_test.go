// Copyright 2022 The LUCI Authors.
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

package analyzedtestvariants

import (
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/server/span"

	spanutil "go.chromium.org/luci/analysis/internal/span"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/analysis/internal/testutil/insert"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	pb "go.chromium.org/luci/analysis/proto/v1"
	. "go.chromium.org/luci/common/testing/assertions"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAnalyzedTestVariantSpan(t *testing.T) {
	Convey(`TestAnalyzedTestVariantSpan`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		realm := "chromium:ci"
		status := atvpb.Status_FLAKY
		now := clock.Now(ctx).UTC()
		ps := []atvpb.Status{
			atvpb.Status_CONSISTENTLY_EXPECTED,
			atvpb.Status_FLAKY,
		}
		puts := []time.Time{
			now.Add(-24 * time.Hour),
			now.Add(-240 * time.Hour),
		}
		builder := "builder"

		tmdBytes, err := proto.Marshal(&pb.TestMetadata{
			Name: "name",
			Location: &pb.TestLocation{
				Repo:     "repo",
				FileName: "filename",
				Line:     456,
			},
		})
		So(err, ShouldBeNil)
		ms := []*spanner.Mutation{
			insert.AnalyzedTestVariant(realm, "ninja://test1", "variantHash1", status,
				map[string]interface{}{
					"Builder":                   builder,
					"StatusUpdateTime":          now.Add(-time.Hour),
					"PreviousStatuses":          ps,
					"PreviousStatusUpdateTimes": puts,
					"TestMetadata":              spanutil.Compressed(tmdBytes),
				}),
			insert.AnalyzedTestVariant(realm, "ninja://test1", "variantHash2", atvpb.Status_HAS_UNEXPECTED_RESULTS, map[string]interface{}{
				"Builder": builder,
			}),
			insert.AnalyzedTestVariant(realm, "ninja://test2", "variantHash1", status,
				map[string]interface{}{
					"Builder": "anotherbuilder",
				}),
			insert.AnalyzedTestVariant(realm, "ninja://test3", "variantHash", status, nil),
			insert.AnalyzedTestVariant(realm, "ninja://test4", "variantHash", atvpb.Status_CONSISTENTLY_EXPECTED,
				map[string]interface{}{
					"Builder": builder,
				}),
			insert.AnalyzedTestVariant("anotherrealm", "ninja://test1", "variantHash1", status,
				map[string]interface{}{
					"Builder": builder,
				}),
		}
		testutil.MustApply(ctx, ms...)

		Convey(`TestReadSummary`, func() {
			ks := spanner.KeySets(
				spanner.Key{realm, "ninja://test1", "variantHash1"},
				spanner.Key{realm, "ninja://test1", "variantHash2"},
				spanner.Key{realm, "ninja://test-not-exists", "variantHash1"},
			)
			atvs := make([]*atvpb.AnalyzedTestVariant, 0)
			err := ReadSummary(span.Single(ctx), ks, func(atv *atvpb.AnalyzedTestVariant) error {
				So(atv.Realm, ShouldEqual, realm)
				atvs = append(atvs, atv)
				return nil
			})
			So(err, ShouldBeNil)
			So(atvs, ShouldResembleProto, []*atvpb.AnalyzedTestVariant{
				{
					Realm:       realm,
					TestId:      "ninja://test1",
					VariantHash: "variantHash1",
					Status:      atvpb.Status_FLAKY,
					Tags:        []*pb.StringPair{},
					TestMetadata: &pb.TestMetadata{
						Name: "name",
						Location: &pb.TestLocation{
							Repo:     "repo",
							FileName: "filename",
							Line:     456,
						},
					},
				},
				{
					Realm:       realm,
					TestId:      "ninja://test1",
					VariantHash: "variantHash2",
					Status:      atvpb.Status_HAS_UNEXPECTED_RESULTS,
					Tags:        []*pb.StringPair{},
				},
			})
		})

		Convey(`TestReadStatusHistory`, func() {
			exp := &StatusHistory{
				Status:                    status,
				StatusUpdateTime:          now.Add(-time.Hour),
				PreviousStatuses:          ps,
				PreviousStatusUpdateTimes: puts,
			}

			si, enqTime, err := ReadStatusHistory(span.Single(ctx), spanner.Key{realm, "ninja://test1", "variantHash1"})
			So(err, ShouldBeNil)
			So(si, ShouldResemble, exp)
			So(enqTime, ShouldResemble, spanner.NullTime{})
		})

		Convey(`TestQueryTestVariantsByBuilder`, func() {
			atvs := make([]*atvpb.AnalyzedTestVariant, 0)
			err := QueryTestVariantsByBuilder(span.Single(ctx), realm, builder, func(atv *atvpb.AnalyzedTestVariant) error {
				atvs = append(atvs, atv)
				return nil
			})
			So(err, ShouldBeNil)
			So(len(atvs), ShouldEqual, 2)
		})
	})
}
