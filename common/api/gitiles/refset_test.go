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
	"regexp"
	"testing"

	"github.com/golang/mock/gomock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
)

func TestRefSet(t *testing.T) {
	t.Parallel()

	ftt.Run("RefSet", t, func(t *ftt.Test) {
		wr := NewRefSet([]string{
			`refs/heads/master`,
			`regexp:refs/branch-heads/\d+\.\d+`,
			`regexp:refs/missing/many.+`,
			`refs/missing/exact`,
		})

		t.Run("explicit refs", func(t *ftt.Test) {
			assert.Loosely(t, wr.Has("refs/heads/master"), should.BeTrue)
			assert.Loosely(t, wr.Has("refs/heads/foo"), should.BeFalse)
		})

		t.Run("regexp refs", func(t *ftt.Test) {
			assert.Loosely(t, wr.Has("refs/branch-heads/1.12"), should.BeTrue)
			assert.Loosely(t, wr.Has("refs/branch-heads/1.12.123"), should.BeFalse)
		})

		t.Run("resolve ref tips", func(t *ftt.Test) {
			ctx := context.Background()
			ctl := gomock.NewController(t)
			mockClient := mock_gitiles.NewMockGitilesClient(ctl)

			commonExpect := func() {
				mockClient.EXPECT().Refs(gomock.Any(), proto.MatcherEqual(&gitiles.RefsRequest{
					Project: "project", RefsPath: "refs/heads",
				})).Return(
					&gitiles.RefsResponse{Revisions: map[string]string{
						"refs/heads/master": "01234567",
						"refs/heads/foobar": "89abcdef",
					}}, nil,
				)
				mockClient.EXPECT().Refs(gomock.Any(), proto.MatcherEqual(&gitiles.RefsRequest{
					Project: "project", RefsPath: "refs/missing",
				})).Return(&gitiles.RefsResponse{}, nil)
			}

			t.Run("normal", func(t *ftt.Test) {
				commonExpect()
				mockClient.EXPECT().Refs(gomock.Any(), proto.MatcherEqual(&gitiles.RefsRequest{
					Project: "project", RefsPath: "refs/branch-heads",
				})).Return(
					&gitiles.RefsResponse{Revisions: map[string]string{
						"refs/branch-heads/1.9":      "cafedead",
						"refs/branch-heads/1.10":     "deadcafe",
						"refs/branch-heads/1.11.123": "deadbeef",
					}}, nil,
				)

				refTips, missing, err := wr.Resolve(ctx, mockClient, "project")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, refTips, should.Resemble(map[string]string{
					"refs/heads/master":      "01234567",
					"refs/branch-heads/1.9":  "cafedead",
					"refs/branch-heads/1.10": "deadcafe",
				}))
				assert.Loosely(t, missing, should.Resemble([]string{`refs/missing/exact`, `regexp:refs/missing/many.+`}))
			})

			t.Run("failed RPCs", func(t *ftt.Test) {
				commonExpect()
				mockClient.EXPECT().Refs(gomock.Any(), proto.MatcherEqual(&gitiles.RefsRequest{
					Project: "project", RefsPath: "refs/branch-heads",
				})).Return(
					nil, errors.New("foobar"),
				)

				_, _, err := wr.Resolve(ctx, mockClient, "project")
				assert.Loosely(t, err.Error(), should.ContainSubstring("foobar"))
			})
		})
	})

	ftt.Run("ValidateRefSet", t, func(t *ftt.Test) {
		ctx := &validation.Context{Context: context.Background()}

		t.Run("plain refs", func(t *ftt.Test) {
			t.Run("too few slashes", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`refs/foo`})
				assert.Loosely(t, ctx.Finalize().Error(), should.ContainSubstring(
					`fewer than 2 slashes in ref "refs/foo"`))
			})
			t.Run("does not start with refs/", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`foo/bar/baz`})
				assert.Loosely(t, ctx.Finalize().Error(), should.ContainSubstring(
					`ref must start with 'refs/' not "foo/bar/baz"`))
			})
			t.Run("valid", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`refs/heads/master`})
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
		})

		t.Run("regexp refs", func(t *ftt.Test) {
			t.Run("starts with ^ or ends with $", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:^refs/branch-heads/\d+\.\d+$`})
				assert.Loosely(t, ctx.Finalize().Error(), should.ContainSubstring(
					`^ and $ qualifiers are added automatically, please remove them`))
			})
			t.Run("invalid regexp", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:([{`})
				assert.Loosely(t, ctx.Finalize().Error(), should.ContainSubstring(`invalid regexp`))
			})
			t.Run("matches single ref only is fine", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:refs/h[e]ad(s)/m[a]ster`})
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
			t.Run("fewer than 2 slashes in literal prefix", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:refs/branch[-_]heads/\d+\/\d+`})
				assert.Loosely(t, ctx.Finalize().Error(), should.ContainSubstring(
					`fewer than 2 slashes in literal prefix "refs/branch"`))
			})
			t.Run("does not start with refs/", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:foo/branch-heads/\d+\/\d+`})
				assert.Loosely(t, ctx.Finalize().Error(), should.ContainSubstring(
					`literal prefix "foo/branch-heads/" must start with "refs/"`))
			})
			t.Run("non-trivial ref prefix is supported", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:refs/foo\.bar/\d+`})
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
			t.Run("not-trivial literal prefix is supported", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:refs/branch-heads/(6\.8|6\.9)\.\d+`})
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
			t.Run("valid", func(t *ftt.Test) {
				ValidateRefSet(ctx, []string{`regexp:refs/branch-heads/\d+\.\d+`})
				assert.Loosely(t, ctx.Finalize(), should.BeNil)
			})
		})
	})

	ftt.Run("smoke test of LiteralPrefix not working as expected", t, func(t *ftt.Test) {
		r := "refs/heads/\\d+\\.\\d+.\\d"
		l1, _ := regexp.MustCompile(r).LiteralPrefix()
		l2, _ := regexp.MustCompile("^" + r + "$").LiteralPrefix()
		assert.Loosely(t, l1, should.Match("refs/heads/"))
		assert.Loosely(t, l2, should.BeBlank) // See https://github.com/golang/go/issues/30425
		NewRefSet([]string{"regexp:" + r})
	})
}
