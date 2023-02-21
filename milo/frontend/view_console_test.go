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

package frontend

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/frontend/ui"
)

func TestRenderOncallers(t *testing.T) {
	t.Parallel()
	ctx := gaetesting.TestingContext()
	Convey("Oncall fetching works", t, func() {
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

		Convey("Fetch failed", func() {
			oncallConfig := config.Oncall{
				Url: server.URL + "?name=bad",
			}
			result, err := getOncallData(ctx, &oncallConfig)
			So(err, ShouldBeNil)
			So(result.Oncallers, ShouldEqual, template.HTML(`ERROR: Fetching oncall failed`))
		})
		Convey("Fetch succeeded", func() {
			oncallConfig := config.Oncall{
				Name: "Good rotation",
				Url:  server.URL + "?name=good",
			}
			result, err := getOncallData(ctx, &oncallConfig)
			So(err, ShouldBeNil)
			So(result.Name, ShouldEqual, "Good rotation")
			So(result.Oncallers, ShouldEqual, template.HTML(`foo`))
		})
	})

	Convey("Rendering oncallers works", t, func() {
		Convey("Legacy trooper format", func() {
			oncallConfig := config.Oncall{
				Name: "Legacy trooper",
				Url:  "http://fake-rota.appspot.com/legacy/trooper.json",
			}
			Convey("No-one oncall", func() {
				response := ui.Oncall{
					Primary:     "",
					Secondaries: []string{},
				}

				So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(`&lt;none&gt;`))
			})
			Convey("No-one oncall with 'None' string", func() {
				response := ui.Oncall{
					Primary:     "None",
					Secondaries: []string{},
				}

				So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(`None`))
			})
			Convey("Primary only", func() {
				Convey("Googler", func() {
					response := ui.Oncall{
						Primary:     "foo@google.com",
						Secondaries: []string{},
					}

					So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(`foo`))
				})
				Convey("Non-Googler", func() {
					response := ui.Oncall{
						Primary:     "foo@example.com",
						Secondaries: []string{},
					}

					So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(
						`foo<span style="display:none">ohnoyoudont</span>@example.com`))
				})
			})
			Convey("Primary and secondaries", func() {
				response := ui.Oncall{
					Primary:     "foo@google.com",
					Secondaries: []string{"bar@google.com", "baz@example.com"},
				}

				So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(
					`foo (primary), bar (secondary), baz<span style="display:none">ohnoyoudont</span>@example.com (secondary)`))
			})
		})
		Convey("Email-only format", func() {
			oncallConfig := config.Oncall{
				Name: "Legacy trooper",
				Url:  "http://fake-rota.appspot.com/legacy/trooper.json",
			}

			Convey("No-one oncall", func() {
				response := ui.Oncall{
					Emails: []string{},
				}

				So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(`&lt;none&gt;`))
			})

			Convey("Primary only", func() {
				Convey("Googler", func() {
					response := ui.Oncall{
						Emails: []string{"foo@google.com"},
					}

					So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(`foo`))
				})
				Convey("Non-Googler", func() {
					response := ui.Oncall{
						Emails: []string{"foo@example.com"},
					}

					So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(
						`foo<span style="display:none">ohnoyoudont</span>@example.com`))
				})
			})

			Convey("Primary and secondaries", func() {
				Convey("Primary/secondary labeling disabled", func() {
					response := ui.Oncall{
						Emails: []string{"foo@google.com", "bar@google.com", "baz@example.com"},
					}

					So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(
						`foo, bar, baz<span style="display:none">ohnoyoudont</span>@example.com`))
				})
				Convey("Primary/secondary labeling enabled", func() {
					oncallConfig := config.Oncall{
						Name:                       "Legacy trooper",
						Url:                        "http://fake-rota.appspot.com/legacy/trooper.json",
						ShowPrimarySecondaryLabels: true,
					}
					response := ui.Oncall{
						Emails: []string{"foo@google.com", "bar@google.com", "baz@example.com"},
					}

					So(renderOncallers(&oncallConfig, &response), ShouldEqual, template.HTML(
						`foo (primary), bar (secondary), baz<span style="display:none">ohnoyoudont</span>@example.com (secondary)`))
				})
			})
		})
	})
}
