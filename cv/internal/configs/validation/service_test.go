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

package validation

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
)

func TestListenerConfigValidation(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate Config", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		vctx := &validation.Context{Context: ctx}
		configSet := "services/luci-change-verifier"
		path := "listener-settings.cfg"

		t.Run("Loading bad proto", func(t *ftt.Test) {
			content := []byte(` bad: "config" `)
			assert.NoErr(t, validateListenerSettings(vctx, configSet, path, content))
			assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring("unknown field"))
		})

		t.Run("OK", func(t *ftt.Test) {
			cfg := []byte(`
				gerrit_subscriptions {
					host: "chromium-review.googlesource.com"
					receive_settings: {
						num_goroutines: 100
						max_outstanding_messages: 5000
					}
					message_format: JSON
				}
				gerrit_subscriptions {
					host: "pigweed-review.googlesource.com"
					subscription_id: "pigweed_gerrit"
					receive_settings: {
						num_goroutines: 100
						max_outstanding_messages: 5000
					}
					message_format: PROTO_BINARY
				}
			`)
			t.Run("fully loaded", func(t *ftt.Test) {
				assert.NoErr(t, validateListenerSettings(vctx, configSet, path, cfg))
				assert.NoErr(t, vctx.Finalize())
			})
			t.Run("empty", func(t *ftt.Test) {
				assert.NoErr(t, validateListenerSettings(vctx, configSet, path, []byte{}))
				assert.NoErr(t, vctx.Finalize())
			})
		})

		t.Run("Fail", func(t *ftt.Test) {
			t.Run("multiple subscriptions for the same host", func(t *ftt.Test) {
				cfg := []byte(`
					gerrit_subscriptions {
						host: "example.org"
						subscription_id: "sub"
						message_format: JSON
					}
					gerrit_subscriptions {
						host: "example.org"
						subscription_id: "sub2"
						message_format: JSON
					}
				`)
				assert.NoErr(t, validateListenerSettings(vctx, configSet, path, cfg))
				assert.ErrIsLike(t, vctx.Finalize(), `already exists for host "example.org"`)
			})

			t.Run("invalid disabled_project_regexps", func(t *ftt.Test) {
				cfg := []byte(`
					disabled_project_regexps: "(123"
				`)
				assert.NoErr(t, validateListenerSettings(vctx, configSet, path, cfg))
				assert.ErrIsLike(t, vctx.Finalize(), "missing closing")
			})

			t.Run("invalid message_format", func(t *ftt.Test) {
				cfg := []byte(`
					gerrit_subscriptions {
						host: "example.org"
					}
				`)
				assert.NoErr(t, validateListenerSettings(vctx, configSet, path, cfg))
				assert.Loosely(t, vctx.Finalize().Error(), should.ContainSubstring(
					"message_format): must be specified"))
			})
		})

		t.Run("watched repo", func(t *ftt.Test) {
			pc := &prjcfg.ProjectConfig{
				Project:          "chromium",
				ConfigGroupNames: []string{"main"},
				Hash:             "sha256:deadbeef",
				Enabled:          true,
			}
			cpb := &cfgpb.ConfigGroup{
				Name: "main",
				Gerrit: []*cfgpb.ConfigGroup_Gerrit{
					{
						Url: "https://chromium-review.googlesoure.com/foo",
						Projects: []*cfgpb.ConfigGroup_Gerrit_Project{
							{Name: "cr/src1"},
							{Name: "cr/src2"},
						},
					},
				},
			}
			cg := &prjcfg.ConfigGroup{
				Project: prjcfg.ProjectConfigKey(ctx, pc.Project),
				ID:      prjcfg.MakeConfigGroupID(pc.Hash, "main"),
				Content: cpb,
			}
			assert.NoErr(t, datastore.Put(ctx, pc, cg))

			t.Run("missing gerrit_subscriptions", func(t *ftt.Test) {
				cfg := []byte(`
					gerrit_subscriptions {
						host: "pigweed-review.googlesource.com"
						subscription_id: "pigweed_gerrit"
						receive_settings: {
							num_goroutines: 100
							max_outstanding_messages: 5000
						}
						message_format: PROTO_BINARY
					}
				`)

				t.Run("fails", func(t *ftt.Test) {
					assert.NoErr(t, validateListenerSettings(vctx, configSet, path, cfg))
					assert.ErrIsLike(t, vctx.Finalize(), "there is no gerrit_subscriptions")
				})

				t.Run("succeeeds if disabled in listener settings", func(t *ftt.Test) {
					cfg := append(cfg, []byte(`
						disabled_project_regexps: "chromium"
					`)...)
					assert.NoErr(t, validateListenerSettings(vctx, configSet, path, cfg))
					assert.NoErr(t, vctx.Finalize())
				})
			})
		})
	})
}
