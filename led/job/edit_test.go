// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package job

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	bbpb "go.chromium.org/luci/buildbucket/proto"
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

func TestPriority(t *testing.T) {
	t.Parallel()

	runCases(t, `Priority`, []testCase{
		{
			name: "negative",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Priority(-1)
				})
				So(jd.Info().Priority(), ShouldEqual, 0)
			},
		},

		{
			name: "set",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Priority(100)
				})
				So(jd.Info().Priority(), ShouldEqual, 100)
			},
		},

		{
			name: "reset",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.Priority(100)
				})
				SoEdit(jd, func(je Editor) {
					je.Priority(200)
				})
				So(jd.Info().Priority(), ShouldEqual, 200)
			},
		},
	})
}

func TestSwarmingHostname(t *testing.T) {
	t.Parallel()

	runCases(t, `SwarmingHostname`, []testCase{
		{
			name: "empty",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.SwarmingHostname("")
				})
				So(jd.Info().SwarmingHostname(), ShouldEqual, "")
			},
		},

		{
			name: "set",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.SwarmingHostname("example.com")
				})
				So(jd.Info().SwarmingHostname(), ShouldEqual, "example.com")
			},
		},

		{
			name: "reset",
			fn: func(jd *Definition) {
				SoEdit(jd, func(je Editor) {
					je.SwarmingHostname("example.com")
				})
				SoEdit(jd, func(je Editor) {
					je.SwarmingHostname("other.example.com")
				})
				So(jd.Info().SwarmingHostname(), ShouldEqual, "other.example.com")
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

func TestGerritChange(t *testing.T) {
	t.Parallel()

	mkChange := func(project string) *bbpb.GerritChange {
		return &bbpb.GerritChange{
			Host:     "host.example.com",
			Project:  project,
			Change:   1,
			Patchset: 1,
		}
	}

	runCases(t, `GerritChange`, []testCase{
		{
			name:   "nil",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.AddGerritChange(nil)
					je.RemoveGerritChange(nil)
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldBeEmpty)
			},
		},

		{
			name:   "new",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("project"))
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldResemble, []*bbpb.GerritChange{
					mkChange("project"),
				})
			},
		},

		{
			name:   "dupe",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("project"))
					je.AddGerritChange(mkChange("project"))
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldResemble, []*bbpb.GerritChange{
					mkChange("project"),
				})
			},
		},

		{
			name:   "remove",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("a"))
					je.AddGerritChange(mkChange("b"))
					je.AddGerritChange(mkChange("c"))

					je.RemoveGerritChange(mkChange("b"))
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldResemble, []*bbpb.GerritChange{
					mkChange("a"),
					mkChange("c"),
				})
			},
		},

		{
			name:   "remove (noop)",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("a"))

					je.RemoveGerritChange(mkChange("b"))
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldResemble, []*bbpb.GerritChange{
					mkChange("a"),
				})
			},
		},
	})
}

func TestTaskPayload(t *testing.T) {
	t.Parallel()

	runCases(t, `TaskPayload`, []testCase{
		{
			name:   "empty",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayload("", "", "")
				})
				pkg, vers, path := jd.HighLevelInfo().TaskPayload()
				So(pkg, ShouldEqual, "")
				So(vers, ShouldEqual, "")
				So(path, ShouldEqual, ".")
			},
		},

		{
			name:   "isolate",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayload("", "", "some/path")
				})
				pkg, vers, path := jd.HighLevelInfo().TaskPayload()
				So(pkg, ShouldEqual, "")
				So(vers, ShouldEqual, "")
				So(path, ShouldEqual, "some/path")
			},
		},

		{
			name:   "cipd",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayload("pkgname", "latest", "some/path")
				})
				pkg, vers, path := jd.HighLevelInfo().TaskPayload()
				So(pkg, ShouldEqual, "pkgname")
				So(vers, ShouldEqual, "latest")
				So(path, ShouldEqual, "some/path")
			},
		},
	})
}
