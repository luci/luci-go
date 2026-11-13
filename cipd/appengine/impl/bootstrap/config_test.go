// Copyright 2021 The LUCI Authors.
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

package bootstrap

import (
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/cfgcache"

	api "go.chromium.org/luci/cipd/api/config/v1"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ctx, ts := cfgcache.TestingContext(memory.Use(t.Context()))
	datastore.GetTestable(ctx).Consistent(true)
	cfg := &Config{cachedCfg.Register(ts.Registry)}
	assert.NoErr(t, ts.Initialize(ctx))

	ts.SetConfig("services/my-app", "bootstrap.cfg", &api.BootstrapConfigFile{
		BootstrapConfig: []*api.BootstrapConfig{
			{Prefix: "pkg/a/specific"},
			{Prefix: "pkg/a"},
		},
	})
	assert.NoErr(t, ts.IngestToStorage(ctx))
	assert.NoErr(t, ts.RefreshInMemory(ctx))

	bs, err := cfg.Get(ctx, "pkg/a/specific/zzz")
	assert.NoErr(t, err)
	assert.That(t, bs.Prefix, should.Equal("pkg/a/specific"))

	bs, err = cfg.Get(ctx, "pkg/a/specific")
	assert.NoErr(t, err)
	assert.Loosely(t, bs.Prefix, should.Equal("pkg/a/specific"))

	bs, err = cfg.Get(ctx, "pkg/a/another")
	assert.NoErr(t, err)
	assert.Loosely(t, bs.Prefix, should.Equal("pkg/a"))
}
