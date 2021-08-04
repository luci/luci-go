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
	. "go.chromium.org/luci/common/testing/assertions"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	api "go.chromium.org/luci/swarming/proto/api"
)

func TestClearCurrentIsolated(t *testing.T) {
	t.Parallel()

	runCases(t, `ClearCurrentIsolated`, []testCase{
		{
			name: "basic with isolate input",
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
				So(iso, ShouldResemble, &isolated{})
			},
		},
		{
			name: "basic with rbe-cas input",
			fn: func(jd *Definition) {
				jd.CasUserPayload = &api.CASReference{
					CasInstance: "instance",
					Digest: &api.Digest{
						Hash:      "hash",
						SizeBytes: 1,
					},
				}
				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					if slc.Properties == nil {
						slc.Properties = &api.TaskProperties{}
					}
					slc.Properties.CasInputRoot = &api.CASReference{
						CasInstance: "instance",
						Digest: &api.Digest{
							Hash:      "hash",
							SizeBytes: 1,
						},
					}
				}
				SoEdit(jd, func(je Editor) {
					je.ClearCurrentIsolated()
				})

				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					So(slc.Properties.CasInputRoot, ShouldBeNil)
				}

				iso, err := jd.Info().CurrentIsolated()
				So(err, ShouldBeNil)
				So(iso, ShouldResemble, &isolated{})
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

func TestPriority(t *testing.T) {
	t.Parallel()

	runCases(t, `Priority`, []testCase{
		{
			name: "negative",
			fn: func(jd *Definition) {
				So(jd.Edit(func(je Editor) {
					je.Priority(-1)
				}), ShouldErrLike, "negative Priority")
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
				So(jd.Edit(func(je Editor) {
					je.SwarmingHostname("")
				}), ShouldErrLike, "empty SwarmingHostname")
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

func TestTaskName(t *testing.T) {
	t.Parallel()

	runCases(t, `TaskName`, []testCase{
		{
			name: "set",
			fn: func(jd *Definition) {
				if jd.GetBuildbucket() == nil && jd.GetSwarming().GetTask() == nil {
					So(jd.Info().TaskName(), ShouldResemble, "")
				} else {
					So(jd.Info().TaskName(), ShouldResemble, "default-task-name")
				}

				SoEdit(jd, func(je Editor) {
					je.TaskName("")
				})
				So(jd.Info().TaskName(), ShouldResemble, "")

				SoEdit(jd, func(je Editor) {
					je.TaskName("something")
				})
				So(jd.Info().TaskName(), ShouldResemble, "something")
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

func TestExperiments(t *testing.T) {
	t.Parallel()

	runCases(t, `Experiments`, []testCase{
		{
			name:   "full",
			skipSW: true,
			fn: func(jd *Definition) {
				So(jd.HighLevelInfo().Experiments(), ShouldHaveLength, 0)

				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Experiments(map[string]bool{
						"exp1":         true,
						"exp2":         true,
						"delMissingOK": false,
					})
				})
				So(jd.HighLevelInfo().Experiments(), ShouldResemble, []string{"exp1", "exp2"})

				SoHLEdit(jd, func(je HighLevelEditor) {
					je.Experiments(map[string]bool{
						"exp1": false,
					})
				})
				So(jd.HighLevelInfo().Experiments(), ShouldResemble, []string{"exp2"})
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
				So(jd.HighLevelInfo().GerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
					mkChange("project"),
				})
			},
		},

		{
			name:   "clear",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("project"))
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
					mkChange("project"),
				})
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.ClearGerritChanges()
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldBeEmpty)
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
				So(jd.HighLevelInfo().GerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
					mkChange("project"),
				})
			},
		},

		{
			name:   "remove",
			skipSW: true,
			fn: func(jd *Definition) {
				bb := jd.GetBuildbucket()
				bb.EnsureBasics()
				bb.BbagentArgs.Build.Input.GerritChanges = []*bbpb.GerritChange{
					mkChange("a"),
					mkChange("b"),
					mkChange("c"),
				}
				So(jd.HighLevelInfo().GerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
					mkChange("a"),
					mkChange("b"),
					mkChange("c"),
				})

				SoHLEdit(jd, func(je HighLevelEditor) {
					je.RemoveGerritChange(mkChange("b"))
				})
				So(jd.HighLevelInfo().GerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
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
				So(jd.HighLevelInfo().GerritChanges(), ShouldResembleProto, []*bbpb.GerritChange{
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

func TestTaskPayload(t *testing.T) {
	t.Parallel()

	runCases(t, `TaskPayload`, []testCase{
		{
			name:   "empty",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("", "")
					je.TaskPayloadPath("")
					je.TaskPayloadCmd(nil)
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				So(pkg, ShouldEqual, "")
				So(vers, ShouldEqual, "")
				So(hli.TaskPayloadPath(), ShouldEqual, "")
				So(hli.TaskPayloadCmd(), ShouldResemble, []string{"luciexe"})
			},
		},

		{
			name:   "isolate",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("", "")
					je.TaskPayloadPath("some/path")
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				So(pkg, ShouldEqual, "")
				So(vers, ShouldEqual, "")
				So(hli.TaskPayloadPath(), ShouldEqual, "some/path")
			},
		},

		{
			name:   "cipd",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("pkgname", "latest")
					je.TaskPayloadPath("some/path")
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				So(pkg, ShouldEqual, "pkgname")
				So(vers, ShouldEqual, "latest")
				So(hli.TaskPayloadPath(), ShouldEqual, "some/path")
			},
		},

		{
			name:   "cipd latest",
			skipSW: true,
			fn: func(jd *Definition) {
				SoHLEdit(jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("pkgname", "")
					je.TaskPayloadPath("some/path")
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				So(pkg, ShouldEqual, "pkgname")
				So(vers, ShouldEqual, "latest")
				So(hli.TaskPayloadPath(), ShouldEqual, "some/path")
			},
		},
	})
}
