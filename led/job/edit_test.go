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

	"go.chromium.org/luci/buildbucket"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"
)

func TestClearCurrentIsolated(t *testing.T) {
	t.Parallel()

	runCases(t, `ClearCurrentIsolated`, []testCase{
		{
			name: "basic with rbe-cas input",
			fn: func(t *ftt.Test, jd *Definition) {
				if jd.GetSwarming() != nil {
					jd.GetSwarming().CasUserPayload = &swarmingpb.CASReference{
						CasInstance: "instance",
						Digest: &swarmingpb.Digest{
							Hash:      "hash",
							SizeBytes: 1,
						},
					}
				}
				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					if slc.Properties == nil {
						slc.Properties = &swarmingpb.TaskProperties{}
					}
					slc.Properties.CasInputRoot = &swarmingpb.CASReference{
						CasInstance: "instance",
						Digest: &swarmingpb.Digest{
							Hash:      "hash",
							SizeBytes: 1,
						},
					}
				}
				MustEdit(t, jd, func(je Editor) {
					je.ClearCurrentIsolated()
				})

				for _, slc := range jd.GetSwarming().GetTask().GetTaskSlices() {
					assert.Loosely(t, slc.Properties.CasInputRoot, should.BeNil)
				}

				iso, err := jd.Info().CurrentIsolated()
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, iso, should.BeNil)
			},
		},
	})
}

func TestEnv(t *testing.T) {
	t.Parallel()

	runCases(t, `Env`, []testCase{
		{
			name: "empty",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Env(nil)
				})
				env, err := jd.Info().Env()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.BeEmpty)
			},
		},

		{
			name:        "new",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Env(map[string]string{
						"KEY": "VALUE",
						"DEL": "", // noop
					})
				})
				env, err := jd.Info().Env()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.Resemble(map[string]string{
					"KEY": "VALUE",
				}))
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Env(map[string]string{
						"KEY": "VALUE",
					})
				})
				MustEdit(t, jd, func(je Editor) {
					je.Env(map[string]string{
						"OTHER": "NEW_VAL",
						"DEL":   "", // noop
					})
				})
				env, err := jd.Info().Env()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.Resemble(map[string]string{
					"KEY":   "VALUE",
					"OTHER": "NEW_VAL",
				}))
			},
		},

		{
			name:        "del",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Env(map[string]string{
						"KEY": "VALUE",
					})
				})
				MustEdit(t, jd, func(je Editor) {
					je.Env(map[string]string{
						"OTHER": "NEW_VAL",
						"KEY":   "", // delete
					})
				})
				env, err := jd.Info().Env()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.Resemble(map[string]string{
					"OTHER": "NEW_VAL",
				}))
			},
		},
	})
}

func TestPriority(t *testing.T) {
	t.Parallel()

	runCases(t, `Priority`, []testCase{
		{
			name: "negative",
			fn: func(t *ftt.Test, jd *Definition) {
				assert.Loosely(t, jd.Edit(func(je Editor) {
					je.Priority(-1)
				}), should.ErrLike("negative Priority"))
			},
		},

		{
			name: "set",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Priority(100)
				})
				assert.Loosely(t, jd.Info().Priority(), should.Equal(100))
			},
		},

		{
			name: "reset",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Priority(100)
				})
				MustEdit(t, jd, func(je Editor) {
					je.Priority(200)
				})
				assert.Loosely(t, jd.Info().Priority(), should.Equal(200))
			},
		},
	})
}

func TestSwarmingHostname(t *testing.T) {
	t.Parallel()

	runCases(t, `SwarmingHostname`, []testCase{
		{
			name: "empty",
			fn: func(t *ftt.Test, jd *Definition) {
				assert.Loosely(t, jd.Edit(func(je Editor) {
					je.SwarmingHostname("")
				}), should.ErrLike("empty SwarmingHostname"))
			},
		},

		{
			name: "set",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.SwarmingHostname("example.com")
				})
				assert.Loosely(t, jd.Info().SwarmingHostname(), should.Equal("example.com"))
			},
		},

		{
			name: "reset",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.SwarmingHostname("example.com")
				})
				MustEdit(t, jd, func(je Editor) {
					je.SwarmingHostname("other.example.com")
				})
				assert.Loosely(t, jd.Info().SwarmingHostname(), should.Equal("other.example.com"))
			},
		},
	})
}

func TestTaskName(t *testing.T) {
	t.Parallel()

	runCases(t, `TaskName`, []testCase{
		{
			name: "set",
			fn: func(t *ftt.Test, jd *Definition) {
				if jd.GetBuildbucket() == nil && jd.GetSwarming().GetTask() == nil {
					assert.Loosely(t, jd.Info().TaskName(), should.BeBlank)
				} else {
					assert.Loosely(t, jd.Info().TaskName(), should.Match("default-task-name"))
				}

				MustEdit(t, jd, func(je Editor) {
					je.TaskName("")
				})
				assert.Loosely(t, jd.Info().TaskName(), should.BeBlank)

				MustEdit(t, jd, func(je Editor) {
					je.TaskName("something")
				})
				assert.Loosely(t, jd.Info().TaskName(), should.Match("something"))
			},
		},
	})
}

func TestPrefixPathEnv(t *testing.T) {
	t.Parallel()

	runCases(t, `PrefixPathEnv`, []testCase{
		{
			name: "empty",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.PrefixPathEnv(nil)
				})
				env, err := jd.Info().PrefixPathEnv()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.BeEmpty)
			},
		},

		{
			name:        "add",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.PrefixPathEnv([]string{"some/path", "other/path"})
				})
				env, err := jd.Info().PrefixPathEnv()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.Resemble([]string{
					"some/path", "other/path",
				}))
			},
		},

		{
			name:        "remove",
			skipSWEmpty: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.PrefixPathEnv([]string{"some/path", "other/path", "third"})
				})
				MustEdit(t, jd, func(je Editor) {
					je.PrefixPathEnv([]string{"!other/path"})
				})
				env, err := jd.Info().PrefixPathEnv()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, env, should.Resemble([]string{
					"some/path", "third",
				}))
			},
		},
	})
}

func TestTags(t *testing.T) {
	t.Parallel()

	runCases(t, `Tags`, []testCase{
		{
			name: "empty",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Tags(nil)
				})
				assert.Loosely(t, jd.Info().Tags(), should.BeEmpty)
			},
		},

		{
			name: "add",
			fn: func(t *ftt.Test, jd *Definition) {
				MustEdit(t, jd, func(je Editor) {
					je.Tags([]string{"other:value", "key:value"})
				})
				assert.Loosely(t, jd.Info().Tags(), should.Resemble([]string{
					"key:value", "other:value",
				}))
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
			fn: func(t *ftt.Test, jd *Definition) {
				assert.Loosely(t, jd.HighLevelInfo().Experimental(), should.BeFalse)
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Experimental(true)
				})
				assert.Loosely(t, jd.HighLevelInfo().Experimental(), should.BeTrue)
				assert.Loosely(t, jd.HighLevelInfo().Experiments(), should.Resemble([]string{buildbucket.ExperimentNonProduction}))

				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Experimental(false)
				})
				assert.Loosely(t, jd.HighLevelInfo().Experimental(), should.BeFalse)
				assert.Loosely(t, jd.HighLevelInfo().Experiments(), should.HaveLength(0))
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
			fn: func(t *ftt.Test, jd *Definition) {
				assert.Loosely(t, jd.HighLevelInfo().Experiments(), should.HaveLength(0))

				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Experiments(map[string]bool{
						"exp1":         true,
						"exp2":         true,
						"delMissingOK": false,
					})
				})
				er := jd.GetBuildbucket().BbagentArgs.Build.Infra.Buildbucket.ExperimentReasons
				assert.Loosely(t, jd.HighLevelInfo().Experiments(), should.Resemble([]string{"exp1", "exp2"}))
				assert.Loosely(t, er, should.Resemble(map[string]bbpb.BuildInfra_Buildbucket_ExperimentReason{
					"exp1": bbpb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
					"exp2": bbpb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
				}))
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Experiments(map[string]bool{
						"exp1": false,
					})
				})
				assert.Loosely(t, jd.HighLevelInfo().Experiments(), should.Resemble([]string{"exp2"}))
				assert.Loosely(t, er, should.Resemble(map[string]bbpb.BuildInfra_Buildbucket_ExperimentReason{
					"exp2": bbpb.BuildInfra_Buildbucket_EXPERIMENT_REASON_REQUESTED,
				}))
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
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Properties(nil, false)
				})
				props, err := jd.HighLevelInfo().Properties()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, props, should.BeEmpty)
			},
		},

		{
			name:   "add",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"key":    `"value"`,
						"other":  `{"something": [1, 2, "subvalue"]}`,
						"delete": "", // noop
					}, false)
				})
				props, err := jd.HighLevelInfo().Properties()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, props, should.Resemble(map[string]string{
					"key":   `"value"`,
					"other": `{"something":[1,2,"subvalue"]}`,
				}))
			},
		},

		{
			name:   "add (auto)",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"json":    `{"thingy": "kerplop"}`,
						"literal": `{I am a banana}`,
						"delete":  "", // noop
					}, true)
				})
				props, err := jd.HighLevelInfo().Properties()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, props, should.Resemble(map[string]string{
					"json":    `{"thingy":"kerplop"}`,
					"literal": `"{I am a banana}"`, // string now
				}))
			},
		},

		{
			name:   "delete",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"key": `"value"`,
					}, false)
				})
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.Properties(map[string]string{
						"key": "",
					}, false)
				})
				props, err := jd.HighLevelInfo().Properties()
				assert.Loosely(t, err, should.ErrLike(nil))
				assert.Loosely(t, props, should.BeEmpty)
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
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.AddGerritChange(nil)
					je.RemoveGerritChange(nil)
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.BeEmpty)
			},
		},

		{
			name:   "new",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("project"))
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.Resemble([]*bbpb.GerritChange{
					mkChange("project"),
				}))
			},
		},

		{
			name:   "clear",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("project"))
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.Resemble([]*bbpb.GerritChange{
					mkChange("project"),
				}))
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.ClearGerritChanges()
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.BeEmpty)
			},
		},

		{
			name:   "dupe",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("project"))
					je.AddGerritChange(mkChange("project"))
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.Resemble([]*bbpb.GerritChange{
					mkChange("project"),
				}))
			},
		},

		{
			name:   "remove",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				bb := jd.GetBuildbucket()
				bb.EnsureBasics()
				bb.BbagentArgs.Build.Input.GerritChanges = []*bbpb.GerritChange{
					mkChange("a"),
					mkChange("b"),
					mkChange("c"),
				}
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.Resemble([]*bbpb.GerritChange{
					mkChange("a"),
					mkChange("b"),
					mkChange("c"),
				}))

				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.RemoveGerritChange(mkChange("b"))
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.Resemble([]*bbpb.GerritChange{
					mkChange("a"),
					mkChange("c"),
				}))
			},
		},

		{
			name:   "remove (noop)",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.AddGerritChange(mkChange("a"))

					je.RemoveGerritChange(mkChange("b"))
				})
				assert.Loosely(t, jd.HighLevelInfo().GerritChanges(), should.Resemble([]*bbpb.GerritChange{
					mkChange("a"),
				}))
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
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.GitilesCommit(nil)
				})
				assert.Loosely(t, jd.HighLevelInfo().GitilesCommit(), should.BeNil)
			},
		},

		{
			name:   "add",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.GitilesCommit(&bbpb.GitilesCommit{Id: "deadbeef"})
				})
				assert.Loosely(t, jd.HighLevelInfo().GitilesCommit(), should.Resemble(&bbpb.GitilesCommit{
					Id: "deadbeef",
				}))
			},
		},

		{
			name:   "del",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.GitilesCommit(&bbpb.GitilesCommit{Id: "deadbeef"})
				})
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.GitilesCommit(nil)
				})
				assert.Loosely(t, jd.HighLevelInfo().GitilesCommit(), should.BeNil)
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
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("", "")
					je.TaskPayloadPath("")
					je.TaskPayloadCmd(nil)
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				assert.Loosely(t, pkg, should.BeEmpty)
				assert.Loosely(t, vers, should.BeEmpty)
				assert.Loosely(t, hli.TaskPayloadPath(), should.BeEmpty)
				assert.Loosely(t, hli.TaskPayloadCmd(), should.Resemble([]string{"luciexe"}))
			},
		},

		{
			name:   "isolate",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("", "")
					je.TaskPayloadPath("some/path")
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				assert.Loosely(t, pkg, should.BeEmpty)
				assert.Loosely(t, vers, should.BeEmpty)
				assert.Loosely(t, hli.TaskPayloadPath(), should.Equal("some/path"))
			},
		},

		{
			name:   "cipd",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("pkgname", "latest")
					je.TaskPayloadPath("some/path")
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				assert.Loosely(t, pkg, should.Equal("pkgname"))
				assert.Loosely(t, vers, should.Equal("latest"))
				assert.Loosely(t, hli.TaskPayloadPath(), should.Equal("some/path"))
			},
		},

		{
			name:   "cipd latest",
			skipSW: true,
			fn: func(t *ftt.Test, jd *Definition) {
				MustHLEdit(t, jd, func(je HighLevelEditor) {
					je.TaskPayloadSource("pkgname", "")
					je.TaskPayloadPath("some/path")
				})
				hli := jd.HighLevelInfo()
				pkg, vers := hli.TaskPayloadSource()
				assert.Loosely(t, pkg, should.Equal("pkgname"))
				assert.Loosely(t, vers, should.Equal("latest"))
				assert.Loosely(t, hli.TaskPayloadPath(), should.Equal("some/path"))
			},
		},
	})
}
