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

package job

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	api "go.chromium.org/luci/swarming/proto/api"
	swarmingpb "go.chromium.org/luci/swarming/proto/api"
)

func TestClearCurrentIsolated(t *testing.T) {
	t.Parallel()

	runCases(t, `ClearCurrentIsolated`, []testCase{
		{
			name: "basic",
			fn: func(jd *Definition) {
				jd.UserPayload = &api.CASTree{
					Digest:    "deadbeef",
					Namespace: "namespace",
					Server:    "server",
				}
				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					if slc.Properties == nil {
						slc.Properties = &api.TaskProperties{}
					}
					slc.Properties.CasInputs = &api.CASTree{
						Digest:    "deadbeef",
						Namespace: "namespace",
						Server:    "server",
					}
				}
				SoEdit(jd, func(je Editor) {
					je.ClearCurrentIsolated()
				})

				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					So(slc.Properties.CasInputs, ShouldBeNil)
				}

				iso, err := jd.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(iso, ShouldResemble, &swarmingpb.CASTree{
					Namespace: "namespace",
					Server:    "server",
				})

			},
		},
	})
}

func TestEnv(t *testing.T) {
	t.Parallel()

	runCases(t, `Env`, []testCase{
		{
			name: "empty",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Env(nil)
				})
				So(must(jd.Info().Env()), ShouldBeEmpty)
			},
		},

		{
			name:        "new",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Env(map[string]string{
						"KEY": "VALUE",
						"DEL": "", // noop
					})
				})
				So(must(jd.Info().Env()), ShouldResemble, map[string]string{
					"KEY": "VALUE",
				})
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Env(map[string]string{
						"KEY": "VALUE",
					})
				})
				SoEdit(jd, func(je Editor) {
					je.Env(map[string]string{
						"OTHER": "NEW_VAL",
						"DEL":   "", // noop
					})
				})
				So(must(jd.Info().Env()), ShouldResemble, map[string]string{
					"KEY":   "VALUE",
					"OTHER": "NEW_VAL",
				})
			},
		},

		{
			name:        "del",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Env(map[string]string{
						"KEY": "VALUE",
					})
				})
				SoEdit(jd, func(je Editor) {
					je.Env(map[string]string{
						"OTHER": "NEW_VAL",
						"KEY":   "", // delete
					})
				})
				So(must(jd.Info().Env()), ShouldResemble, map[string]string{
					"OTHER": "NEW_VAL",
				})
			},
		},
	})
}

func TestPrefixPathEnv(t *testing.T) {
	t.Parallel()

	runCases(t, `PrefixPathEnv`, []testCase{
		{
			name: "empty",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.PrefixPathEnv(nil)
				})
				So(must(jd.Info().PrefixPathEnv()), ShouldBeEmpty)
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.PrefixPathEnv([]string{"some/path", "other/path"})
				})
				So(must(jd.Info().PrefixPathEnv()), ShouldResemble, []string{
					"some/path", "other/path",
				})
			},
		},

		{
			name:        "remove",
			skipSWEmpty: true,
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.PrefixPathEnv([]string{"some/path", "other/path", "third"})
				})
				SoEdit(jd, func(je Editor) {
					je.PrefixPathEnv([]string{"!other/path"})
				})
				So(must(jd.Info().PrefixPathEnv()), ShouldResemble, []string{
					"some/path", "third",
				})
			},
		},
	})
}

func TestTags(t *testing.T) {
	t.Parallel()

	runCases(t, `Tags`, []testCase{
		{
			name: "empty",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Tags(nil)
				})
				So(jd.Info().Tags(), ShouldBeEmpty)
			},
		},

		{
			name: "add",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Tags([]string{"other:value", "key:value"})
				})
				So(jd.Info().Tags(), ShouldResemble, []string{
					"key:value", "other:value",
				})
			},
		},
	})
}
