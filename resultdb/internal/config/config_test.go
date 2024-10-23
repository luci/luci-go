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

package config

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestServiceConfig(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	ftt.Run("Set and get config", t, func(t *ftt.Test) {
		cfg := CreatePlaceHolderServiceConfig()
		err := SetServiceConfig(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)
		cfgFromGet, err := GetServiceConfig(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Resemble(cfg))
	})
}
