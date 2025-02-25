// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"testing"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestBuildSet(t *testing.T) {
	t.Parallel()

	ftt.Run("Gerrit", t, func(t *ftt.Test) {
		t.Run("ParseBuildSet", func(t *ftt.Test) {
			actual := ParseBuildSet("patch/gerrit/chromium-review.googlesource.com/678507/3")
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   678507,
				Patchset: 3,
			}))
		})
		t.Run("BuildSet", func(t *ftt.Test) {
			bs := GerritBuildSet(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   678507,
				Patchset: 3,
			})
			assert.Loosely(t, bs, should.Equal("patch/gerrit/chromium-review.googlesource.com/678507/3"))
		})
	})

	ftt.Run("Gitiles", t, func(t *ftt.Test) {
		t.Run("ParseBuildSet", func(t *ftt.Test) {
			actual := ParseBuildSet("commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			assert.Loosely(t, actual, should.Match(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Id:      "b7a757f457487cd5cfe2dae83f65c5bc10e288b7",
			}))
		})
		t.Run("not sha1", func(t *ftt.Test) {
			bs := ParseBuildSet("commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/non-sha1")
			assert.Loosely(t, bs, should.BeNil)
		})

		t.Run("no host", func(t *ftt.Test) {
			bs := ParseBuildSet("commit/gitiles//infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			assert.Loosely(t, bs, should.BeNil)
		})
		t.Run("no plus", func(t *ftt.Test) {
			bs := ParseBuildSet("commit/gitiles//infra/luci/luci-go/b7a757f457487cd5cfe2dae83f65c5bc10e288b7")
			assert.Loosely(t, bs, should.BeNil)
		})
		t.Run("BuildSet", func(t *ftt.Test) {
			bs := GitilesBuildSet(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Id:      "b7a757f457487cd5cfe2dae83f65c5bc10e288b7",
			})
			assert.Loosely(t, bs, should.Equal("commit/gitiles/chromium.googlesource.com/infra/luci/luci-go/+/b7a757f457487cd5cfe2dae83f65c5bc10e288b7"))
		})
	})
}

func TestStringPairs(t *testing.T) {
	t.Parallel()

	ftt.Run("StringPairs", t, func(t *ftt.Test) {
		m := strpair.Map{}
		m.Add("a", "1")
		m.Add("a", "2")
		m.Add("b", "1")

		pairs := StringPairs(m)
		assert.Loosely(t, pairs, should.Match([]*pb.StringPair{
			{Key: "a", Value: "1"},
			{Key: "a", Value: "2"},
			{Key: "b", Value: "1"},
		}))
	})

	ftt.Run("StringPairMap", t, func(t *ftt.Test) {
		expected := make(strpair.Map)
		var stringPairs []*pb.StringPair
		assert.Loosely(t, StringPairMap(stringPairs), should.Match(expected))

		stringPairs = []*pb.StringPair{}
		assert.Loosely(t, StringPairMap(stringPairs), should.Match(expected))

		stringPairs = []*pb.StringPair{
			{Key: "a", Value: "1"},
			{Key: "a", Value: "2"},
			{Key: "b", Value: "1"},
		}
		expected = strpair.ParseMap([]string{"a:1", "a:2", "b:1"})
		assert.Loosely(t, StringPairMap(stringPairs), should.Match(expected))
	})
}
