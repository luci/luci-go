// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
