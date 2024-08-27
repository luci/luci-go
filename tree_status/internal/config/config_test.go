// Copyright 2024 The LUCI Authors.
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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	configpb "go.chromium.org/luci/tree_status/proto/config"
)

func TestGetTreeConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("GetTreeConfig", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		testConfig := TestConfig()
		err := SetConfig(ctx, testConfig)
		assert.Loosely(t, err, should.BeNil)

		t.Run("returns existing config", func(t *ftt.Test) {
			treeCfg, err := GetTreeConfig(ctx, "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, treeCfg, should.Match(&configpb.Tree{
				Name:           "chromium",
				Projects:       []string{"chromium"},
				UseDefaultAcls: true,
			}))
		})

		t.Run("errors for non-existing config", func(t *ftt.Test) {
			_, err := GetTreeConfig(ctx, "some-non-existing-tree")
			assert.Loosely(t, err, should.ErrLikeError(ErrNotFoundTreeConfig))
		})
	})
}
