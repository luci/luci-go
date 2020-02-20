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

	Convey(`ClearCurrentIsolated`, t, func() {
		jd := &Definition{
			UserPayload: &api.CASTree{
				Digest:    "deadbeef",
				Namespace: "namespace",
				Server:    "server",
			},
			JobType: &Definition_Swarming{Swarming: &Swarming{
				Task: &api.TaskRequest{
					TaskSlices: []*api.TaskSlice{
						{Properties: &api.TaskProperties{CasInputs: &api.CASTree{
							Digest:    "deadbeef",
							Namespace: "namespace",
							Server:    "server",
						}}},
						{Properties: &api.TaskProperties{}},
					},
				},
			}},
		}

		err := jd.Edit(func(je Editor) {
			je.ClearCurrentIsolated()
		})
		So(err, ShouldBeNil)

		So(jd.GetUserPayload().GetDigest(), ShouldBeEmpty)
		So(jd.GetSwarming().Task.TaskSlices[0].Properties.CasInputs, ShouldBeNil)
		So(jd.GetSwarming().Task.TaskSlices[1].Properties.CasInputs, ShouldBeNil)

		iso, err := jd.Info().CurrentIsolated()
		So(err, ShouldBeNil)
		So(iso, ShouldResemble, &swarmingpb.CASTree{
			Namespace: "namespace",
			Server:    "server",
		})
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

func TestExperimental(t *testing.T) {
	t.Parallel()

	runCases(t, `Experimental`, []testCase{
		{
			name:   "full",
			skipSW: true,
			fn: func(jd *Definition) {
				So(jd.HighLevelInfo().Experimental(), ShouldBeFalse)
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Experimental(true)
				})
				So(jd.HighLevelInfo().Experimental(), ShouldBeTrue)

				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Experimental(false)
				})
				So(jd.HighLevelInfo().Experimental(), ShouldBeFalse)
			},
		},
	})
}

func TestProperties(t *testing.T) {
	t.Parallel()

	runCases(t, `Properties`, []testCase{
		{
			name:   "nil",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Properties(nil, false)
				})
				So(must(jd.HighLevelInfo().Properties()), ShouldBeEmpty)
			},
		},

		{
			name:   "add",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"key":    `"value"`,
						"other":  `{"something": [1, 2, "subvalue"]}`,
						"delete": "", // noop
					}, false)
				})
				So(must(jd.HighLevelInfo().Properties()), ShouldResemble, map[string]string{
					"key":   `"value"`,
					"other": `{"something":[1,2,"subvalue"]}`,
				})
			},
		},

		{
			name:   "add (auto)",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"json":    `{"thingy": "kerplop"}`,
						"literal": `{I am a banana}`,
						"delete":  "", // noop
					}, true)
				})
				So(must(jd.HighLevelInfo().Properties()), ShouldResemble, map[string]string{
					"json":    `{"thingy":"kerplop"}`,
					"literal": `"{I am a banana}"`, // string now
				})
			},
		},

		{
			name:   "delete",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"key": `"value"`,
					}, false)
				})
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"key": "",
					}, false)
				})
				So(must(jd.HighLevelInfo().Properties()), ShouldBeEmpty)
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

func TestGitilesCommit(t *testing.T) {
	t.Parallel()

	runCases(t, `GitilesChange`, []testCase{
		{
			name:   "nil",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.GitilesCommit(nil)
				})
				So(jd.HighLevelInfo().GitilesCommit(), ShouldBeNil)
			},
		},

		{
			name:   "add",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.GitilesCommit(&bbpb.GitilesCommit{Id: "deadbeef"})
				})
				So(jd.HighLevelInfo().GitilesCommit(), ShouldResemble, &bbpb.GitilesCommit{
					Id: "deadbeef",
				})
			},
		},

		{
			name:   "del",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.GitilesCommit(&bbpb.GitilesCommit{Id: "deadbeef"})
				})
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.GitilesCommit(nil)
				})
				So(jd.HighLevelInfo().GitilesCommit(), ShouldBeNil)
			},
		},
	})
}
