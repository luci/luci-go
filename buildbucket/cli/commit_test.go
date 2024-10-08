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

package cli

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestParseCommit(t *testing.T) {
	ftt.Run("ParseCommit", t, func(t *ftt.Test) {

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/7a63166bfab5de38ddb2cb8e29aca756bdc2a28d", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/7a63166bfab5de38ddb2cb8e29aca756bdc2a28d")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeFalse)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "",
				Id:      "7a63166bfab5de38ddb2cb8e29aca756bdc2a28d",
			}))
		})

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/x", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/x")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeFalse)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/heads/x",
				Id:      "",
			}))
		})

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/tags/10.0.1", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/tags/10.0.1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeTrue)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/tags/10.0.1",
				Id:      "",
			}))
		})

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/x/y", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/heads/x/y")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeTrue)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/heads/x",
				Id:      "",
			}))
		})

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/branch-heads/x", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/branch-heads/x")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeTrue)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/branch-heads/x",
				Id:      "",
			}))
		})

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/main", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/main")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeTrue)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/heads/main",
				Id:      "",
			}))
		})

		t.Run("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/x", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium.googlesource.com/infra/luci/luci-go/+/refs/x")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeTrue)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/x",
				Id:      "",
			}))
		})

		t.Run("https://chromium-foo.googlesource.com/infra/luci/luci-go/+/refs/x", func(t *ftt.Test) {
			actual, confirm, err := parseCommit("https://chromium-foo.googlesource.com/infra/luci/luci-go/+/refs/x")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, confirm, should.BeTrue)
			assert.Loosely(t, actual, should.Resemble(&pb.GitilesCommit{
				Host:    "chromium-foo.googlesource.com",
				Project: "infra/luci/luci-go",
				Ref:     "refs/x",
				Id:      "",
			}))
		})
	})
}
