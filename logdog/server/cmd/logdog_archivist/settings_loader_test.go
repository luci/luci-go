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

package main

import (
	"bytes"
	"context"
	"testing"

	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/logdog/server/archivist"
	srvcfg "go.chromium.org/luci/logdog/server/config"
)

func TestGetSettingsLoader(t *testing.T) {
	t.Parallel()

	ftt.Run("GetSettingsLoader", t, func(t *ftt.Test) {
		project := "test-project"
		lucicfg := map[config.Set]cfgmem.Files{
			"services/${appid}": {
				"services.cfg": `coordinator { admin_auth_group: "a" }`,
			},
			config.Set("projects/" + project): {},
		}
		ctx := memory.Use(context.Background())
		ctx = srvcfg.WithStore(ctx, &srvcfg.Store{})
		getSettings := func(ctx context.Context, s string) *archivist.Settings {
			lucicfg[config.Set("projects/"+project)]["${appid}.cfg"] = s
			ctx = cfgclient.Use(ctx, cfgmem.New(lucicfg))
			assert.Loosely(t, srvcfg.Sync(ctx), should.BeNil)

			f := GetSettingsLoader("", &CommandLineFlags{})
			settings, err := f(ctx, project)
			assert.Loosely(t, err, should.BeNil)
			return settings
		}

		var buf bytes.Buffer
		buf.WriteString(`archive_gs_bucket: "a"`)

		t.Run("works with ArchiveIndexConfig", func(t *ftt.Test) {
			buf.WriteString(`
				archive_index_config: {
					stream_range: 3,
					prefix_range: 2,
					byte_range: 1,
				},`)
			assert.Loosely(t, getSettings(ctx, buf.String()), should.Resemble(&archivist.Settings{
				GSBase:           gs.Path("gs://a"),
				IndexStreamRange: 3,
				IndexPrefixRange: 2,
				IndexByteRange:   1,
			}))
		})

		t.Run("works with CloudLoggingConfig", func(t *ftt.Test) {
			buf.WriteString(`
				cloud_logging_config: {
					destination: "foo",
				},
			`)
			assert.Loosely(t, getSettings(ctx, buf.String()), should.Resemble(&archivist.Settings{
				GSBase:                gs.Path("gs://a"),
				CloudLoggingProjectID: "foo",
			}))
		})
	})
}
