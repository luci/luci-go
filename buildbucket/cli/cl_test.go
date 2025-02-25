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

func TestParseCL(t *testing.T) {
	ftt.Run("ParseCL", t, func(t *ftt.Test) {

		t.Run("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/#/c/infra/luci/luci-go/+/1541677/7")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677/7/buildbucket/cmd/bb/base_command.go")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Project:  "infra/luci/luci-go",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/c/1541677/7", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/1541677/7")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   1541677,
				Patchset: 7,
			}))
		})

		t.Run("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677", func(t *ftt.Test) {
			actual, err := parseCL("https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/1541677")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:    "chromium-review.googlesource.com",
				Project: "infra/luci/luci-go",
				Change:  1541677,
			}))
		})

		t.Run("crrev.com/c/123", func(t *ftt.Test) {
			actual, err := parseCL("crrev.com/c/123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:   "chromium-review.googlesource.com",
				Change: 123,
			}))
		})

		t.Run("crrev.com/c/123/4", func(t *ftt.Test) {
			actual, err := parseCL("crrev.com/c/123/4")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chromium-review.googlesource.com",
				Change:   123,
				Patchset: 4,
			}))
		})

		t.Run("crrev.com/i/123", func(t *ftt.Test) {
			actual, err := parseCL("crrev.com/i/123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:   "chrome-internal-review.googlesource.com",
				Change: 123,
			}))
		})

		t.Run("https://crrev.com/i/123", func(t *ftt.Test) {
			actual, err := parseCL("https://crrev.com/i/123")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:   "chrome-internal-review.googlesource.com",
				Change: 123,
			}))
		})

		t.Run("https://chrome-internal-review.googlesource.com/c/src/+/1/2", func(t *ftt.Test) {
			actual, err := parseCL("https://chrome-internal-review.googlesource.com/c/src/+/1/2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(&pb.GerritChange{
				Host:     "chrome-internal-review.googlesource.com",
				Project:  "src",
				Change:   1,
				Patchset: 2,
			}))
		})
	})
}
