// Copyright 2020 The LUCI Authors.
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

package cfgclient

import (
	"context"
	"testing"

	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"
)

func TestHelpers(t *testing.T) {
	t.Parallel()

	ftt.Run("ProjectsWithConfig works", t, func(t *ftt.Test) {
		ctx := Use(context.Background(), memory.New(map[config.Set]memory.Files{
			"projects/a": {"f.cfg": ""},
			"projects/b": {},
			"projects/c": {"f.cfg": ""},
			"services/s": {"f.cfg": ""},
		}))
		p, err := ProjectsWithConfig(ctx, "f.cfg")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, p, should.Match([]string{"a", "c"}))
	})

	ftt.Run("Get works", t, func(t *ftt.Test) {
		ctx := Use(context.Background(), memory.New(map[config.Set]memory.Files{
			"projects/a": {
				"cfg.text":   `blah`,
				"cfg.textpb": `name: "blah"`,
				"cfg.jsonpb": `{"name": "blah"}`,
			},
		}))

		t.Run("Bytes", func(t *ftt.Test) {
			dst := []byte(nil)
			meta := config.Meta{}
			assert.Loosely(t, Get(ctx, "projects/a", "cfg.text", Bytes(&dst), &meta), should.BeNil)
			assert.Loosely(t, dst, should.Match([]byte("blah")))
			assert.Loosely(t, meta.ContentHash, should.NotBeBlank)
		})

		t.Run("String", func(t *ftt.Test) {
			dst := ""
			meta := config.Meta{}
			assert.Loosely(t, Get(ctx, "projects/a", "cfg.text", String(&dst), &meta), should.BeNil)
			assert.Loosely(t, dst, should.Equal("blah"))
			assert.Loosely(t, meta.ContentHash, should.NotBeBlank)
		})

		t.Run("ProtoText", func(t *ftt.Test) {
			dst := configPB.ProjectCfg{}
			meta := config.Meta{}
			assert.Loosely(t, Get(ctx, "projects/a", "cfg.textpb", ProtoText(&dst), &meta), should.BeNil)
			assert.Loosely(t, &dst, should.Match(&configPB.ProjectCfg{Name: "blah"}))
			assert.Loosely(t, meta.ContentHash, should.NotBeBlank)
		})

		t.Run("ProtoJSON", func(t *ftt.Test) {
			dst := configPB.ProjectCfg{}
			meta := config.Meta{}
			assert.Loosely(t, Get(ctx, "projects/a", "cfg.jsonpb", ProtoJSON(&dst), &meta), should.BeNil)
			assert.Loosely(t, &dst, should.Match(&configPB.ProjectCfg{Name: "blah"}))
			assert.Loosely(t, meta.ContentHash, should.NotBeBlank)
		})

		t.Run("Meta only", func(t *ftt.Test) {
			meta := config.Meta{}
			assert.Loosely(t, Get(ctx, "projects/a", "cfg.text", nil, &meta), should.BeNil)
			assert.Loosely(t, meta.ContentHash, should.NotBeBlank)
		})

		t.Run("Presence only", func(t *ftt.Test) {
			assert.Loosely(t, Get(ctx, "projects/a", "cfg.text", nil, nil), should.BeNil)
			assert.Loosely(t, Get(ctx, "projects/a", "missing", nil, nil), should.Equal(config.ErrNoConfig))
		})
	})
}
