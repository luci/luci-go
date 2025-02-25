// Copyright 2015 The LUCI Authors.
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

package catalog

import (
	"context"
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/impl/resolving"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/config/vars"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func TestRegisterTaskManagerAndFriends(t *testing.T) {
	t.Parallel()

	ftt.Run("RegisterTaskManager works", t, func(t *ftt.Test) {
		c := New()
		assert.Loosely(t, c.RegisterTaskManager(fakeTaskManager{t: t}), should.BeNil)
		assert.Loosely(t, c.GetTaskManager(&messages.NoopTask{}), should.NotBeNilInterface)
		assert.Loosely(t, c.GetTaskManager(&messages.UrlFetchTask{}), should.BeNil)
		assert.Loosely(t, c.GetTaskManager(nil), should.BeNil)
	})

	ftt.Run("RegisterTaskManager bad proto type", t, func(t *ftt.Test) {
		c := New()
		assert.Loosely(t, c.RegisterTaskManager(brokenTaskManager{}), should.ErrLike("expecting pointer to a struct"))
	})

	ftt.Run("RegisterTaskManager twice", t, func(t *ftt.Test) {
		c := New()
		assert.Loosely(t, c.RegisterTaskManager(fakeTaskManager{t: t}), should.BeNil)
		assert.Loosely(t, c.RegisterTaskManager(fakeTaskManager{t: t}), should.NotBeNil)
	})
}

func TestProtoValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("validateJobProto works", t, func(t *ftt.Test) {
		c := New().(*catalog)

		call := func(j *messages.Job) error {
			valCtx := &validation.Context{Context: ctx}
			c.validateJobProto(valCtx, j, "some-project:some-realm")
			return valCtx.Finalize()
		}

		c.RegisterTaskManager(fakeTaskManager{t: t})
		assert.Loosely(t, call(&messages.Job{}), should.ErrLike("missing 'id' field'"))
		assert.Loosely(t, call(&messages.Job{Id: "bad'id"}), should.ErrLike("not valid value for 'id' field"))
		assert.Loosely(t, call(&messages.Job{
			Id:   "good id can have spaces and . and - and even ()",
			Noop: &messages.NoopTask{},
		}), should.BeNil)
		assert.Loosely(t, call(&messages.Job{
			Id:       "good",
			Schedule: "blah",
		}), should.ErrLike("not valid value for 'schedule' field"))
		assert.Loosely(t, call(&messages.Job{
			Id:       "good",
			Schedule: "* * * * *",
		}), should.ErrLike("can't find a recognized task definition"))
		assert.Loosely(t, call(&messages.Job{
			Id:               "good",
			Schedule:         "* * * * *",
			Noop:             &messages.NoopTask{},
			TriggeringPolicy: &messages.TriggeringPolicy{Kind: 111111},
		}), should.ErrLike("unrecognized policy kind 111111"))
	})

	ftt.Run("extractTaskProto works", t, func(t *ftt.Test) {
		c := New().(*catalog)
		c.RegisterTaskManager(fakeTaskManager{
			name: "noop",
			task: &messages.NoopTask{},
		})
		c.RegisterTaskManager(fakeTaskManager{
			name: "url fetch",
			task: &messages.UrlFetchTask{},
		})

		t.Run("with TaskDefWrapper", func(t *ftt.Test) {
			msg, err := c.extractTaskProto(ctx, &messages.TaskDefWrapper{
				Noop: &messages.NoopTask{},
			}, "some-project:some-realm")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, msg.(*messages.NoopTask), should.NotBeNil)

			msg, err = c.extractTaskProto(ctx, nil, "some-project:some-realm")
			assert.Loosely(t, err, should.ErrLike("expecting a pointer to proto message"))
			assert.Loosely(t, msg, should.BeNil)

			msg, err = c.extractTaskProto(ctx, &messages.TaskDefWrapper{}, "some-project:some-realm")
			assert.Loosely(t, err, should.ErrLike("can't find a recognized task definition"))
			assert.Loosely(t, msg, should.BeNil)

			msg, err = c.extractTaskProto(ctx, &messages.TaskDefWrapper{
				Noop:     &messages.NoopTask{},
				UrlFetch: &messages.UrlFetchTask{},
			}, "some-project:some-realm")
			assert.Loosely(t, err, should.ErrLike("only one field with task definition must be set"))
			assert.Loosely(t, msg, should.BeNil)
		})

		t.Run("with Job", func(t *ftt.Test) {
			msg, err := c.extractTaskProto(ctx, &messages.Job{
				Id:   "blah",
				Noop: &messages.NoopTask{},
			}, "some-project:some-realm")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, msg.(*messages.NoopTask), should.NotBeNil)

			msg, err = c.extractTaskProto(ctx, &messages.Job{
				Id: "blah",
			}, "some-project:some-realm")
			assert.Loosely(t, err, should.ErrLike("can't find a recognized task definition"))
			assert.Loosely(t, msg, should.BeNil)

			msg, err = c.extractTaskProto(ctx, &messages.Job{
				Id:       "blah",
				Noop:     &messages.NoopTask{},
				UrlFetch: &messages.UrlFetchTask{},
			}, "some-project:some-realm")
			assert.Loosely(t, err, should.ErrLike("only one field with task definition must be set"))
			assert.Loosely(t, msg, should.BeNil)
		})
	})

	ftt.Run("extractTaskProto uses task manager validation", t, func(t *ftt.Test) {
		c := New().(*catalog)
		c.RegisterTaskManager(fakeTaskManager{
			name:            "broken noop",
			validationErr:   errors.New("boo"),
			expectedRealmID: "some-project:some-realm",
		})
		msg, err := c.extractTaskProto(ctx, &messages.TaskDefWrapper{
			Noop: &messages.NoopTask{},
		}, "some-project:some-realm")
		assert.Loosely(t, err, should.ErrLike("boo"))
		assert.Loosely(t, msg, should.BeNil)
	})
}

func TestTaskMarshaling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	ftt.Run("works", t, func(t *ftt.Test) {
		c := New().(*catalog)
		c.RegisterTaskManager(fakeTaskManager{
			name:            "url fetch",
			task:            &messages.UrlFetchTask{},
			expectedRealmID: "some-project:some-realm",
		})

		// Round trip for a registered task.
		blob, err := c.marshalTask(&messages.UrlFetchTask{
			Url: "123",
		})
		assert.Loosely(t, err, should.BeNil)
		task, err := c.UnmarshalTask(ctx, blob, "some-project:some-realm")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, task, should.Match(&messages.UrlFetchTask{
			Url: "123",
		}))

		// Unknown task type.
		_, err = c.marshalTask(&messages.NoopTask{})
		assert.Loosely(t, err, should.ErrLike("unrecognized task definition type *messages.NoopTask"))

		// Once registered, but not anymore.
		c = New().(*catalog)
		_, err = c.UnmarshalTask(ctx, blob, "some-project:some-realm")
		assert.Loosely(t, err, should.ErrLike("can't find a recognized task definition"))
	})
}

func TestConfigReading(t *testing.T) {
	t.Parallel()

	ftt.Run("with mocked config", t, func(t *ftt.Test) {
		ctx := testContext()

		// Fetch configs from memory but resolve ${appid} into "app" to make
		// RevisionURL more realistic.
		vars := &vars.VarSet{}
		vars.Register("appid", func(context.Context) (string, error) {
			return "app", nil
		})
		ctx = cfgclient.Use(ctx, resolving.New(vars, memcfg.New(mockedConfigs)))

		cat := New()
		cat.RegisterTaskManager(fakeTaskManager{
			name: "noop",
			task: &messages.NoopTask{},
		})
		cat.RegisterTaskManager(fakeTaskManager{
			name: "url_fetch",
			task: &messages.UrlFetchTask{},
		})

		t.Run("GetAllProjects works", func(t *ftt.Test) {
			projects, err := cat.GetAllProjects(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projects, should.Match([]string{"broken", "project1", "project2"}))
		})

		t.Run("GetProjectJobs works", func(t *ftt.Test) {
			const expectedRev = "06e505e46c49133cc928fbc244b27b232d7e8010"

			defs, err := cat.GetProjectJobs(ctx, "project1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, defs, should.Match([]Definition{
				{
					JobID:            "project1/noop-job-1",
					RealmID:          "project1:public",
					Revision:         expectedRev,
					RevisionURL:      "https://example.com/view/here/app.cfg",
					Schedule:         "*/10 * * * * * *",
					Task:             []uint8{10, 0},
					TriggeringPolicy: []uint8{16, 4},
				},
				{
					JobID:       "project1/noop-job-2",
					RealmID:     "project1:@legacy",
					Revision:    expectedRev,
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{10, 0},
				},
				{
					JobID:       "project1/urlfetch-job-1",
					RealmID:     "project1:@legacy",
					Revision:    expectedRev,
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{18, 21, 18, 19, 104, 116, 116, 112, 115, 58, 47, 47, 101, 120, 97, 109, 112, 108, 101, 46, 99, 111, 109},
				},
				{
					JobID:            "project1/trigger",
					RealmID:          "project1:@legacy",
					Flavor:           JobFlavorTrigger,
					Revision:         expectedRev,
					RevisionURL:      "https://example.com/view/here/app.cfg",
					Schedule:         "with 30s interval",
					Task:             []uint8{10, 0},
					TriggeringPolicy: []uint8{8, 1, 16, 2},
					TriggeredJobIDs: []string{
						"project1/noop-job-1",
						"project1/noop-job-2",
						"project1/noop-job-3",
					},
				},
			}))
		})

		t.Run("GetProjectJobs filters unknown job IDs in triggers", func(t *ftt.Test) {
			const expectedRev = "3ef040fb696156a96c882837b05f31d2da0ba0f5"

			defs, err := cat.GetProjectJobs(ctx, "project2")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, defs, should.Match([]Definition{
				{
					JobID:       "project2/noop-job-1",
					RealmID:     "project2:@legacy",
					Revision:    expectedRev,
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{10, 0},
				},
				{
					JobID:       "project2/trigger",
					RealmID:     "project2:@legacy",
					Flavor:      2,
					Revision:    expectedRev,
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "with 30s interval",
					Task:        []uint8{10, 0},
					TriggeredJobIDs: []string{
						// No noop-job-2 here!
						"project2/noop-job-1",
					},
				},
			}))
		})

		t.Run("GetProjectJobs unknown project", func(t *ftt.Test) {
			defs, err := cat.GetProjectJobs(ctx, "unknown")
			assert.Loosely(t, defs, should.BeNil)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("GetProjectJobs broken proto", func(t *ftt.Test) {
			defs, err := cat.GetProjectJobs(ctx, "broken")
			assert.Loosely(t, defs, should.BeNil)
			assert.Loosely(t, err, should.NotBeNil)
		})

		t.Run("UnmarshalTask works", func(t *ftt.Test) {
			defs, err := cat.GetProjectJobs(ctx, "project1")
			assert.Loosely(t, err, should.BeNil)

			task, err := cat.UnmarshalTask(ctx, defs[0].Task, defs[0].RealmID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, task, should.Match(&messages.NoopTask{}))

			task, err = cat.UnmarshalTask(ctx, []byte("blarg"), defs[0].RealmID)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, task, should.BeNil)
		})
	})
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()

	catalog := New()
	catalog.RegisterTaskManager(fakeTaskManager{
		name: "noop",
		task: &messages.NoopTask{},
	})
	catalog.RegisterTaskManager(fakeTaskManager{
		name: "url_fetch",
		task: &messages.UrlFetchTask{},
	})

	rules := validation.NewRuleSet()
	rules.Vars.Register("appid", func(context.Context) (string, error) {
		return "luci-scheduler", nil
	})
	catalog.RegisterConfigRules(rules)

	ftt.Run("Patterns are correct", t, func(t *ftt.Test) {
		patterns, err := rules.ConfigPatterns(context.Background())
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(patterns), should.Equal(1))
		assert.Loosely(t, patterns[0].ConfigSet.Match("projects/xyz"), should.BeTrue)
		assert.Loosely(t, patterns[0].Path.Match("luci-scheduler.cfg"), should.BeTrue)
	})

	ftt.Run("Config validation works", t, func(t *ftt.Test) {
		ctx := &validation.Context{Context: testContext()}
		t.Run("correct config file content", func(t *ftt.Test) {
			assert.Loosely(t, rules.ValidateConfig(ctx, "projects/good", "luci-scheduler.cfg", []byte(project3Cfg)), should.BeNil)
			assert.Loosely(t, ctx.Finalize(), should.BeNil)
		})

		t.Run("Config that can't be deserialized", func(t *ftt.Test) {
			assert.Loosely(t, rules.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte("deadbeef")), should.BeNil)
			assert.Loosely(t, ctx.Finalize(), should.NotBeNil)
		})

		t.Run("rejects triggers with unknown references", func(t *ftt.Test) {
			assert.Loosely(t, rules.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte(project2Cfg)), should.BeNil)
			assert.Loosely(t, ctx.Finalize(), should.ErrLike(`referencing unknown job "noop-job-2" in 'triggers' field`))
		})

		t.Run("rejects duplicate ids", func(t *ftt.Test) {
			// job + job
			assert.Loosely(t, rules.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte(`
				job {
					id: "dup"
					noop: { }
				}
				job {
					id: "dup"
					noop: { }
				}
			`)), should.BeNil)
			assert.Loosely(t, ctx.Finalize(), should.ErrLike(`duplicate id "dup"`))

			// job + trigger
			assert.Loosely(t, rules.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte(`
				job {
					id: "dup"
					noop: { }
				}
				trigger {
					id: "dup"
					noop: { }
				}
			`)), should.BeNil)
			assert.Loosely(t, ctx.Finalize(), should.ErrLike(`duplicate id "dup"`))
		})
	})
}

////

type fakeTaskManager struct {
	name string
	task proto.Message

	t testing.TB

	validationErr   error
	expectedRealmID string
}

func (m fakeTaskManager) Name() string {
	if m.name != "" {
		return m.name
	}
	return "testing"
}

func (m fakeTaskManager) ProtoMessageType() proto.Message {
	if m.task != nil {
		return m.task
	}
	return &messages.NoopTask{}
}

func (m fakeTaskManager) Traits() task.Traits {
	return task.Traits{}
}

func (m fakeTaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message, realmID string) {
	assert.Loosely(m.t, msg, should.NotBeNil)
	if m.expectedRealmID != "" {
		assert.Loosely(m.t, realmID, should.Equal(m.expectedRealmID))
	}
	if m.validationErr != nil {
		c.Error(m.validationErr)
	}
}

func (m fakeTaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	assert.Loosely(m.t, ctl.Task(), should.NotBeNil)
	return nil
}

func (m fakeTaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

func (m fakeTaskManager) ExamineNotification(c context.Context, msg *pubsub.PubsubMessage) string {
	return ""
}

func (m fakeTaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

func (m fakeTaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return errors.New("not implemented")
}

func (m fakeTaskManager) GetDebugState(c context.Context, ctl task.ControllerReadOnly) (*internal.DebugManagerState, error) {
	return nil, errors.New("not implemented")
}

type brokenTaskManager struct {
	fakeTaskManager
}

func (b brokenTaskManager) ProtoMessageType() proto.Message {
	return nil
}

////

func testContext() context.Context {
	c := gaetesting.TestingContext()
	c = gologger.StdConfig.Use(c)
	c = logging.SetLevel(c, logging.Debug)
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	c, _, _ = tsmon.WithFakes(c)
	tsmon.GetState(c).SetStore(store.NewInMemory(&target.Task{}))
	return c
}

////

const project1Cfg = `
job {
	id: "noop-job-1"
	schedule: "*/10 * * * * * *"
	realm: "public"

	triggering_policy {
		max_concurrent_invocations: 4
	}

	noop: {}
}

job {
	id: "noop-job-2"
	schedule: "*/10 * * * * * *"

	noop: {}
}

job {
	id: "noop-job-3"
	schedule: "*/10 * * * * * *"
	disabled: true

	noop: {}
}

job {
	id: "urlfetch-job-1"
	schedule: "*/10 * * * * * *"

	url_fetch: {
		url: "https://example.com"
	}
}

trigger {
	id: "trigger"

	triggering_policy {
		kind: GREEDY_BATCHING
		max_concurrent_invocations: 2
	}

	noop: {}

	triggers: "noop-job-1"
	triggers: "noop-job-2"
	triggers: "noop-job-3"
}

# Will be skipped since BuildbucketTask Manager is not registered.
job {
	id: "buildbucket-job"
	schedule: "*/10 * * * * * *"

	buildbucket: {}
}
`

// project2Cfg has a trigger that references non-existing job. It will fail
// the config validation, but will still load by GetProjectJobs. We need this
// behavior since unfortunately some inconsistencies crept in into the configs.
const project2Cfg = `
job {
	id: "noop-job-1"
	schedule: "*/10 * * * * * *"
	noop: {}
}

trigger {
	id: "trigger"

	noop: {}

	triggers: "noop-job-1"
	triggers: "noop-job-2"  # no such job
}
`

const project3Cfg = `
job {
	id: "noop-job-v2"
	noop: {
		sleep_ms: 1000
	}
}

trigger {
	id: "noop-trigger-v2"

	noop: {
		sleep_ms: 1000
		triggers_count: 2
	}

	triggers: "noop-job-v2"
}
`

var mockedConfigs = map[config.Set]memcfg.Files{
	"projects/project1": {
		"app.cfg": project1Cfg,
	},
	"projects/project2": {
		"app.cfg": project2Cfg,
	},
	"projects/broken": {
		"app.cfg": "broken!!!!111",
	},
}
