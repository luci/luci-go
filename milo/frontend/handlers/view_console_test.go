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

package handlers

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	projectconfigpb "go.chromium.org/luci/milo/proto/projectconfig"
)

func TestRenderOncallers(t *testing.T) {
	t.Parallel()
	ctx := gaetesting.TestingContext()
	ftt.Run("Oncall fetching works", t, func(t *ftt.Test) {
		serveMux := http.NewServeMux()
		serverResponse := func(w http.ResponseWriter, r *http.Request) {
			name := r.URL.Query()["name"][0]
			if name == "bad" {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			res := map[string]any{
				"emails": []string{"foo"},
			}
			bytes, _ := json.Marshal(res)
			w.Write(bytes)
		}
		serveMux.HandleFunc("/", serverResponse)
		server := httptest.NewServer(serveMux)
		defer server.Close()

		t.Run("Fetch failed", func(t *ftt.Test) {
			oncallConfig := projectconfigpb.Oncall{
				Url: server.URL + "?name=bad",
			}
			result, err := getOncallData(ctx, &oncallConfig)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result.Oncallers, should.Equal(template.HTML(`ERROR: Fetching oncall failed`)))
		})
		t.Run("Fetch succeeded", func(t *ftt.Test) {
			oncallConfig := projectconfigpb.Oncall{
				Name: "Good rotation",
				Url:  server.URL + "?name=good",
			}
			result, err := getOncallData(ctx, &oncallConfig)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result.Name, should.Equal("Good rotation"))
			assert.Loosely(t, result.Oncallers, should.Equal(template.HTML(`foo`)))
		})
	})

	ftt.Run("Rendering oncallers works", t, func(t *ftt.Test) {
		t.Run("Legacy trooper format", func(t *ftt.Test) {
			oncallConfig := projectconfigpb.Oncall{
				Name: "Legacy trooper",
				Url:  "http://fake-rota.appspot.com/legacy/trooper.json",
			}
			t.Run("No-one oncall", func(t *ftt.Test) {
				response := ui.Oncall{
					Primary:     "",
					Secondaries: []string{},
				}

				assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(`&lt;none&gt;`)))
			})
			t.Run("No-one oncall with 'None' string", func(t *ftt.Test) {
				response := ui.Oncall{
					Primary:     "None",
					Secondaries: []string{},
				}

				assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(`None`)))
			})
			t.Run("Primary only", func(t *ftt.Test) {
				t.Run("Googler", func(t *ftt.Test) {
					response := ui.Oncall{
						Primary:     "foo@google.com",
						Secondaries: []string{},
					}

					assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(`foo`)))
				})
				t.Run("Non-Googler", func(t *ftt.Test) {
					response := ui.Oncall{
						Primary:     "foo@example.com",
						Secondaries: []string{},
					}

					assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(
						`foo<span style="display:none">ohnoyoudont</span>@example.com`)))
				})
			})
			t.Run("Primary and secondaries", func(t *ftt.Test) {
				response := ui.Oncall{
					Primary:     "foo@google.com",
					Secondaries: []string{"bar@google.com", "baz@example.com"},
				}

				assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(
					`foo (primary), bar (secondary), baz<span style="display:none">ohnoyoudont</span>@example.com (secondary)`)))
			})
		})
		t.Run("Email-only format", func(t *ftt.Test) {
			oncallConfig := projectconfigpb.Oncall{
				Name: "Legacy trooper",
				Url:  "http://fake-rota.appspot.com/legacy/trooper.json",
			}

			t.Run("No-one oncall", func(t *ftt.Test) {
				response := ui.Oncall{
					Emails: []string{},
				}

				assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(`&lt;none&gt;`)))
			})

			t.Run("Primary only", func(t *ftt.Test) {
				t.Run("Googler", func(t *ftt.Test) {
					response := ui.Oncall{
						Emails: []string{"foo@google.com"},
					}

					assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(`foo`)))
				})
				t.Run("Non-Googler", func(t *ftt.Test) {
					response := ui.Oncall{
						Emails: []string{"foo@example.com"},
					}

					assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(
						`foo<span style="display:none">ohnoyoudont</span>@example.com`)))
				})
			})

			t.Run("Primary and secondaries", func(t *ftt.Test) {
				t.Run("Primary/secondary labeling disabled", func(t *ftt.Test) {
					response := ui.Oncall{
						Emails: []string{"foo@google.com", "bar@google.com", "baz@example.com"},
					}

					assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(
						`foo, bar, baz<span style="display:none">ohnoyoudont</span>@example.com`)))
				})
				t.Run("Primary/secondary labeling enabled", func(t *ftt.Test) {
					oncallConfig := projectconfigpb.Oncall{
						Name:                       "Legacy trooper",
						Url:                        "http://fake-rota.appspot.com/legacy/trooper.json",
						ShowPrimarySecondaryLabels: true,
					}
					response := ui.Oncall{
						Emails: []string{"foo@google.com", "bar@google.com", "baz@example.com"},
					}

					assert.Loosely(t, renderOncallers(&oncallConfig, &response), should.Equal(template.HTML(
						`foo (primary), bar (secondary), baz<span style="display:none">ohnoyoudont</span>@example.com (secondary)`)))
				})
			})
		})
	})
}
