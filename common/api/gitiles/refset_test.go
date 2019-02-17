// Copyright 2018 The LUCI Authors.
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

package gitiles

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/config/validation"
)

func TestRefSet(t *testing.T) {
	t.Parallel()

	Convey("RefSet", t, func() {
		wr := NewRefSet([]string{
			`refs/heads/master`,
			`regexp:refs/branch-heads/\d+\.\d+`,
		})

		Convey("explicit refs", func() {
			So(wr.Has("refs/heads/master"), ShouldBeTrue)
			So(wr.Has("refs/heads/foo"), ShouldBeFalse)
		})

		Convey("regexp refs", func() {
			So(wr.Has("refs/branch-heads/1.12"), ShouldBeTrue)
			So(wr.Has("refs/branch-heads/1.12.123"), ShouldBeFalse)
		})

		Convey("resolve ref tips", func() {
			ctx := context.Background()
			ctl := gomock.NewController(t)
			defer ctl.Finish()
			mockClient := gitiles.NewMockGitilesClient(ctl)

			mockClient.EXPECT().Refs(gomock.Any(), &gitiles.RefsRequest{
				Project: "project", RefsPath: "refs/heads",
			}).Return(
				&gitiles.RefsResponse{Revisions: map[string]string{
					"refs/heads/master": "01234567",
					"refs/heads/foobar": "89abcdef",
				}}, nil,
			)

			Convey("normal", func() {
				mockClient.EXPECT().Refs(gomock.Any(), &gitiles.RefsRequest{
					Project: "project", RefsPath: "refs/branch-heads",
				}).Return(
					&gitiles.RefsResponse{Revisions: map[string]string{
						"refs/branch-heads/1.9":      "cafedead",
						"refs/branch-heads/1.10":     "deadcafe",
						"refs/branch-heads/1.11.123": "deadbeef",
					}}, nil,
				)

				refTips, err := wr.Resolve(ctx, mockClient, "project")
				So(err, ShouldBeNil)
				So(refTips, ShouldResemble, map[string]string{
					"refs/heads/master":      "01234567",
					"refs/branch-heads/1.9":  "cafedead",
					"refs/branch-heads/1.10": "deadcafe",
				})
			})

			Convey("failed RPCs", func() {
				mockClient.EXPECT().Refs(gomock.Any(), &gitiles.RefsRequest{
					Project: "project", RefsPath: "refs/branch-heads",
				}).Return(
					nil, errors.New("foobar"),
				)

				_, err := wr.Resolve(ctx, mockClient, "project")
				So(err.Error(), ShouldContainSubstring, "foobar")
			})
		})
	})

	Convey("ValidateRefSet", t, func() {
		ctx := &validation.Context{Context: context.Background()}

		Convey("plain refs", func() {
			Convey("too few slashes", func() {
				ValidateRefSet(ctx, []string{`refs/foo`})
				So(ctx.Finalize().Error(), ShouldContainSubstring,
					`fewer than 2 slashes in ref "refs/foo"`)
			})
			Convey("does not start with refs/", func() {
				ValidateRefSet(ctx, []string{`foo/bar/baz`})
				So(ctx.Finalize().Error(), ShouldContainSubstring,
					`ref must start with 'refs/' not "foo/bar/baz"`)
			})
			Convey("valid", func() {
				ValidateRefSet(ctx, []string{`refs/heads/master`})
				So(ctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("regexp refs", func() {
			Convey("starts with ^ or ends with $", func() {
				ValidateRefSet(ctx, []string{`regexp:^refs/branch-heads/\d+\.\d+$`})
				So(ctx.Finalize().Error(), ShouldContainSubstring,
					`^ and $ qualifiers are added automatically, please remove them`)
			})
			Convey("invalid regexp", func() {
				ValidateRefSet(ctx, []string{`regexp:([{`})
				So(ctx.Finalize().Error(), ShouldContainSubstring, `invalid regexp`)
			})
			Convey("matches single ref only", func() {
				ValidateRefSet(ctx, []string{`regexp:refs/h[e]ad(s)/m[a]ster`})
				So(ctx.Finalize().Error(), ShouldContainSubstring,
					`matches a single ref only, please use "refs/heads/master" instead`)
			})
			Convey("fewer than 2 slashes in literal prefix", func() {
				ValidateRefSet(ctx, []string{`regexp:refs/branch[-_]heads/\d+\/\d+`})
				So(ctx.Finalize().Error(), ShouldContainSubstring,
					`fewer than 2 slashes in literal prefix "refs/branch"`)
			})
			Convey("does not start with refs/", func() {
				ValidateRefSet(ctx, []string{`regexp:foo/branch-heads/\d+\/\d+`})
				So(ctx.Finalize().Error(), ShouldContainSubstring,
					`literal prefix "foo/branch-heads/" must start with "refs/"`)
			})
			Convey("non-trivial ref prefix is supported", func() {
				ValidateRefSet(ctx, []string{`regexp:refs/foo\.bar/\d+`})
				So(ctx.Finalize(), ShouldBeNil)
			})
			Convey("not-trivial literal prefix is supported", func() {
				ValidateRefSet(ctx, []string{`regexp:refs/branch-heads/(6\.8|6\.9)\.\d+`})
				So(ctx.Finalize(), ShouldBeNil)
			})
			Convey("valid", func() {
				ValidateRefSet(ctx, []string{`regexp:refs/branch-heads/\d+\.\d+`})
				So(ctx.Finalize(), ShouldBeNil)
			})
		})
	})
}

func TestFindNotMatchedRefs(t *testing.T) {
	t.Parallel()

	Convey("FindNotMatchedRefs", t, func() {
		Convey("Simple missing", func() {
			So(FindNotMatchedRefs([]string{"refs/heads/master"}, map[string]string{}),
				ShouldResemble, []string{"refs/heads/master"})
			So(FindNotMatchedRefs([]string{"regexp:refs/heads/.+"}, map[string]string{}),
				ShouldResemble, []string{"regexp:refs/heads/.+"})
		})

		Convey("Simple present", func() {
			So(FindNotMatchedRefs([]string{"refs/heads/master"}, map[string]string{
				"refs/heads/master": "deadbeef",
			}), ShouldResemble, []string{})
			So(FindNotMatchedRefs([]string{"regexp:refs/heads/.+"}, map[string]string{
				"refs/heads/master": "deadbeef",
			}), ShouldResemble, []string{})
		})

		Convey("Hard mix", func() {
			So(FindNotMatchedRefs([]string{
				"refs/heads/master",
				"refs/heads/missing",
				"refs/heads/dups-allowed",
				"refs/heads/dups-allowed",
				"regexp:refs/heads/.+",
				"regexp:refs/digit/\\d",
			}, map[string]string{
				"refs/heads/master":      "deadbeef",
				"refs/digit/not-a-digit": "deadbeef",
			}), ShouldResemble, []string{
				"refs/heads/missing",
				"refs/heads/dups-allowed",
				"refs/heads/dups-allowed",
				"regexp:refs/digit/\\d",
			})
		})
	})
}
