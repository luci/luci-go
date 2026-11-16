// Copyright 2023 The LUCI Authors.
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

package common

import (
	"testing"

	cfgcommonpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGitilesURL(t *testing.T) {
	t.Parallel()

	assert.Loosely(t, GitilesURL(nil), should.BeEmpty)
	assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{}), should.BeEmpty)

	assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{
		Repo: "https://chromium.googlesource.com/infra/infra",
		Ref:  "refs/heads/main",
		Path: "myfile.cfg",
	}), should.Equal("https://chromium.googlesource.com/infra/infra/+/refs/heads/main/myfile.cfg"))

	assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{
		Repo: "https://chromium.googlesource.com/infra/infra",
		Ref:  "refs/heads/main",
	}), should.Equal("https://chromium.googlesource.com/infra/infra/+/refs/heads/main"))

	assert.Loosely(t, GitilesURL(&cfgcommonpb.GitilesLocation{
		Repo: "https://chromium.googlesource.com/infra/infra",
	}), should.Equal("https://chromium.googlesource.com/infra/infra"))
}
