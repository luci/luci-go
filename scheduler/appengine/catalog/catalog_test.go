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
	"time"

	"github.com/golang/protobuf/proto"

	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/config"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/scheduler/appengine/acl"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRegisterTaskManagerAndFriends(t *testing.T) {
	t.Parallel()

	Convey("RegisterTaskManager works", t, func() {
		c := New()
		So(c.RegisterTaskManager(fakeTaskManager{}), ShouldBeNil)
		So(c.GetTaskManager(&messages.NoopTask{}), ShouldNotBeNil)
		So(c.GetTaskManager(&messages.UrlFetchTask{}), ShouldBeNil)
		So(c.GetTaskManager(nil), ShouldBeNil)
	})

	Convey("RegisterTaskManager bad proto type", t, func() {
		c := New()
		So(c.RegisterTaskManager(brokenTaskManager{}), ShouldErrLike, "expecting pointer to a struct")
	})

	Convey("RegisterTaskManager twice", t, func() {
		c := New()
		So(c.RegisterTaskManager(fakeTaskManager{}), ShouldBeNil)
		So(c.RegisterTaskManager(fakeTaskManager{}), ShouldNotBeNil)
	})
}

func TestProtoValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("validateJobProto works", t, func() {
		c := New().(*catalog)

		call := func(j *messages.Job) error {
			valCtx := &validation.Context{Context: ctx}
			c.validateJobProto(valCtx, j)
			return valCtx.Finalize()
		}

		c.RegisterTaskManager(fakeTaskManager{})
		So(call(&messages.Job{}), ShouldErrLike, "missing 'id' field'")
		So(call(&messages.Job{Id: "bad'id"}), ShouldErrLike, "not valid value for 'id' field")
		So(call(&messages.Job{
			Id:   "good id can have spaces and . and - and even ()",
			Noop: &messages.NoopTask{},
		}), ShouldBeNil)
		So(call(&messages.Job{
			Id:       "good",
			Schedule: "blah",
		}), ShouldErrLike, "not valid value for 'schedule' field")
		So(call(&messages.Job{
			Id:       "good",
			Schedule: "* * * * *",
		}), ShouldErrLike, "can't find a recognized task definition")
		So(call(&messages.Job{
			Id:               "good",
			Schedule:         "* * * * *",
			Noop:             &messages.NoopTask{},
			TriggeringPolicy: &messages.TriggeringPolicy{Kind: 111111},
		}), ShouldErrLike, "unrecognized policy kind 111111")
	})

	Convey("extractTaskProto works", t, func() {
		c := New().(*catalog)
		c.RegisterTaskManager(fakeTaskManager{
			name: "noop",
			task: &messages.NoopTask{},
		})
		c.RegisterTaskManager(fakeTaskManager{
			name: "url fetch",
			task: &messages.UrlFetchTask{},
		})

		Convey("with TaskDefWrapper", func() {
			msg, err := c.extractTaskProto(ctx, &messages.TaskDefWrapper{
				Noop: &messages.NoopTask{},
			})
			So(err, ShouldBeNil)
			So(msg.(*messages.NoopTask), ShouldNotBeNil)

			msg, err = c.extractTaskProto(ctx, nil)
			So(err, ShouldErrLike, "expecting a pointer to proto message")
			So(msg, ShouldBeNil)

			msg, err = c.extractTaskProto(ctx, &messages.TaskDefWrapper{})
			So(err, ShouldErrLike, "can't find a recognized task definition")
			So(msg, ShouldBeNil)

			msg, err = c.extractTaskProto(ctx, &messages.TaskDefWrapper{
				Noop:     &messages.NoopTask{},
				UrlFetch: &messages.UrlFetchTask{},
			})
			So(err, ShouldErrLike, "only one field with task definition must be set")
			So(msg, ShouldBeNil)
		})

		Convey("with Job", func() {
			msg, err := c.extractTaskProto(ctx, &messages.Job{
				Id:   "blah",
				Noop: &messages.NoopTask{},
			})
			So(err, ShouldBeNil)
			So(msg.(*messages.NoopTask), ShouldNotBeNil)

			msg, err = c.extractTaskProto(ctx, &messages.Job{
				Id: "blah",
			})
			So(err, ShouldErrLike, "can't find a recognized task definition")
			So(msg, ShouldBeNil)

			msg, err = c.extractTaskProto(ctx, &messages.Job{
				Id:       "blah",
				Noop:     &messages.NoopTask{},
				UrlFetch: &messages.UrlFetchTask{},
			})
			So(err, ShouldErrLike, "only one field with task definition must be set")
			So(msg, ShouldBeNil)
		})
	})

	Convey("extractTaskProto uses task manager validation", t, func() {
		c := New().(*catalog)
		c.RegisterTaskManager(fakeTaskManager{
			name:          "broken noop",
			validationErr: errors.New("boo"),
		})
		msg, err := c.extractTaskProto(ctx, &messages.TaskDefWrapper{
			Noop: &messages.NoopTask{},
		})
		So(err, ShouldErrLike, "boo")
		So(msg, ShouldBeNil)
	})
}

func TestTaskMarshaling(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("works", t, func() {
		c := New().(*catalog)
		c.RegisterTaskManager(fakeTaskManager{
			name: "url fetch",
			task: &messages.UrlFetchTask{},
		})

		// Round trip for a registered task.
		blob, err := c.marshalTask(&messages.UrlFetchTask{
			Url: "123",
		})
		So(err, ShouldBeNil)
		task, err := c.UnmarshalTask(ctx, blob)
		So(err, ShouldBeNil)
		So(task, ShouldResemble, &messages.UrlFetchTask{
			Url: "123",
		})

		// Unknown task type.
		_, err = c.marshalTask(&messages.NoopTask{})
		So(err, ShouldErrLike, "unrecognized task definition type *messages.NoopTask")

		// Once registered, but not anymore.
		c = New().(*catalog)
		_, err = c.UnmarshalTask(ctx, blob)
		So(err, ShouldErrLike, "can't find a recognized task definition")
	})
}

func TestConfigReading(t *testing.T) {
	t.Parallel()

	Convey("with mocked config", t, func() {
		ctx := testContext()
		ctx = testconfig.WithCommonClient(ctx, memcfg.New(mockedConfigs))
		cat := New()
		cat.RegisterTaskManager(fakeTaskManager{
			name: "noop",
			task: &messages.NoopTask{},
		})
		cat.RegisterTaskManager(fakeTaskManager{
			name: "url_fetch",
			task: &messages.UrlFetchTask{},
		})

		// mockedConfigs hardcode 'app' as service name. Verify our mocks are up.
		So(info.TrimmedAppID(ctx), ShouldEqual, "app")

		Convey("GetAllProjects works", func() {
			projects, err := cat.GetAllProjects(ctx)
			So(err, ShouldBeNil)
			So(projects, ShouldResemble, []string{"broken", "project1", "project2"})
		})

		Convey("GetProjectJobs works", func() {
			const expectedRev = "b7f431e8f06f458c638959786b5477a6ae3d565b"

			defs, err := cat.GetProjectJobs(ctx, "project1")
			So(err, ShouldBeNil)
			So(defs, ShouldResemble, []Definition{
				{
					JobID: "project1/noop-job-1",
					Acls: acl.GrantsByRole{
						Readers:    []string{"group:all"},
						Triggerers: []string{},
						Owners:     []string{"group:some-admins"},
					},
					Revision:         expectedRev,
					RevisionURL:      "https://example.com/view/here/app.cfg",
					Schedule:         "*/10 * * * * * *",
					Task:             []uint8{10, 0},
					TriggeringPolicy: []uint8{16, 4},
				},
				{
					JobID: "project1/noop-job-2",
					Acls: acl.GrantsByRole{
						Readers:    []string{"group:all"},
						Triggerers: []string{},
						Owners:     []string{"group:some-admins"},
					},
					Revision:    expectedRev,
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{10, 0},
				},
				{
					JobID: "project1/urlfetch-job-1",
					Acls: acl.GrantsByRole{
						Readers:    []string{"group:all"},
						Triggerers: []string{"group:triggerers"},
						Owners:     []string{"group:debuggers", "group:some-admins"},
					},
					Revision:    expectedRev,
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{18, 21, 18, 19, 104, 116, 116, 112, 115, 58, 47, 47, 101, 120, 97, 109, 112, 108, 101, 46, 99, 111, 109},
				},
				{
					JobID: "project1/trigger",
					Acls: acl.GrantsByRole{
						Readers:    []string{"group:all"},
						Triggerers: []string{},
						Owners:     []string{"group:some-admins"},
					},
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
			})
			So(getConfiguredProjectValue(ctx, "project1"), ShouldBeTrue)
			So(getConfiguredJobsValue(ctx, "project1", "valid"), ShouldEqual, 4)
			So(getConfiguredJobsValue(ctx, "project1", "invalid"), ShouldEqual, 1)
			So(getConfiguredJobsValue(ctx, "project1", "disabled"), ShouldEqual, 1)
		})

		Convey("GetProjectJobs filters unknown job IDs in triggers", func() {
			defs, err := cat.GetProjectJobs(ctx, "project2")
			So(err, ShouldBeNil)
			So(defs, ShouldResemble, []Definition{
				{
					JobID: "project2/noop-job-1",
					Acls: acl.GrantsByRole{
						Owners:     []string{"group:all"},
						Triggerers: []string{},
						Readers:    []string{},
					},
					Revision:    "dcac009130d80e97ec6f380baf4f13b73908ce9a",
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{10, 0},
				},
				{
					JobID: "project2/trigger",
					Acls: acl.GrantsByRole{
						Owners:     []string{"group:all"},
						Triggerers: []string{},
						Readers:    []string{},
					},
					Flavor:      2,
					Revision:    "dcac009130d80e97ec6f380baf4f13b73908ce9a",
					RevisionURL: "https://example.com/view/here/app.cfg",
					Schedule:    "with 30s interval",
					Task:        []uint8{10, 0},
					TriggeredJobIDs: []string{
						// No noop-job-2 here!
						"project2/noop-job-1",
					},
				},
			})
		})

		Convey("GetProjectJobs unknown project", func() {
			defs, err := cat.GetProjectJobs(ctx, "unknown")
			So(defs, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		Convey("GetProjectJobs broken proto", func() {
			defs, err := cat.GetProjectJobs(ctx, "broken")
			So(defs, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(getConfiguredProjectValue(ctx, "broken"), ShouldBeFalse)
		})

		Convey("UnmarshalTask works", func() {
			defs, err := cat.GetProjectJobs(ctx, "project1")
			So(err, ShouldBeNil)

			task, err := cat.UnmarshalTask(ctx, defs[0].Task)
			So(err, ShouldBeNil)
			So(task, ShouldResemble, &messages.NoopTask{})

			task, err = cat.UnmarshalTask(ctx, []byte("blarg"))
			So(err, ShouldNotBeNil)
			So(task, ShouldBeNil)
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

	rules := validation.RuleSet{}
	rules.RegisterVar("appid", func(context.Context) (string, error) {
		return "luci-scheduler", nil
	})
	catalog.RegisterConfigRules(&rules)

	Convey("Patterns are correct", t, func() {
		patterns, err := rules.ConfigPatterns(context.Background())
		So(err, ShouldBeNil)
		So(len(patterns), ShouldEqual, 1)
		So(patterns[0].ConfigSet.Match("projects/xyz"), ShouldBeTrue)
		So(patterns[0].Path.Match("luci-scheduler.cfg"), ShouldBeTrue)
	})

	Convey("Config validation works", t, func() {
		ctx := &validation.Context{Context: testContext()}
		Convey("correct config file content", func() {
			rules.ValidateConfig(ctx, "projects/good", "luci-scheduler.cfg", []byte(project3Cfg))
			So(ctx.Finalize(), ShouldBeNil)
		})

		Convey("Config that can't be deserialized", func() {
			rules.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte("deadbeef"))
			So(ctx.Finalize(), ShouldNotBeNil)
		})

		Convey("semantic errors", func() {
			rules.ValidateConfig(ctx, "projects/validation-error", "luci-scheduler.cfg", []byte(project5Cfg))
			So(ctx.Finalize(), ShouldErrLike, "referencing AclSet \"standard\" which doesn't exist")
		})

		Convey("rejects triggers with unknown references", func() {
			rules.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte(project2Cfg))
			So(ctx.Finalize(), ShouldErrLike, `referencing unknown job "noop-job-2" in 'triggers' field`)
		})
	})
}

////

type fakeTaskManager struct {
	name string
	task proto.Message

	validationErr error
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

func (m fakeTaskManager) ValidateProtoMessage(c *validation.Context, msg proto.Message) {
	So(msg, ShouldNotBeNil)
	if m.validationErr != nil {
		c.Error(m.validationErr)
	}
}

func (m fakeTaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	So(ctl.Task(), ShouldNotBeNil)
	return nil
}

func (m fakeTaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

func (m fakeTaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

func (m fakeTaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return errors.New("not implemented")
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

func getConfiguredProjectValue(ctx context.Context, project string) bool {
	metricValue := tsmon.GetState(ctx).Store().Get(ctx, metricConfigValid, time.Time{}, []interface{}{project})
	return metricValue.(bool)
}

func getConfiguredJobsValue(ctx context.Context, project, status string) int64 {
	metricValue := tsmon.GetState(ctx).Store().Get(ctx, metricConfigJobs, time.Time{}, []interface{}{project, status})
	return metricValue.(int64)
}

////

const project1Cfg = `

acl_sets {
	name: "public"
	acls {
		role: READER
		granted_to: "group:all"
	}
	acls {
		role: OWNER
		granted_to: "group:some-admins"
	}
}

job {
	id: "noop-job-1"
	schedule: "*/10 * * * * * *"
	acl_sets: "public"

	triggering_policy {
		max_concurrent_invocations: 4
	}

	noop: {}
}

job {
	id: "noop-job-2"
	schedule: "*/10 * * * * * *"
	acl_sets: "public"

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
	acl_sets: "public"
	acls {
		role: OWNER
		granted_to: "group:debuggers"
	}
	acls {
		role: TRIGGERER
		granted_to: "group:triggerers"
	}

	url_fetch: {
		url: "https://example.com"
	}
}

trigger {
	id: "trigger"
	acl_sets: "public"

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
acl_sets {
	name: "default"
	acls {
		role: OWNER
		granted_to: "group:all"
	}
}

job {
	id: "noop-job-1"
	acl_sets: "default"
	schedule: "*/10 * * * * * *"
	noop: {}
}

trigger {
	id: "trigger"
	acl_sets: "default"

	noop: {}

	triggers: "noop-job-1"
	triggers: "noop-job-2"  # no such job
}
`

const project3Cfg = `
acl_sets {
	name: "default"
	acls {
		role: READER
		granted_to: "group:all"
	}
	acls {
		role: OWNER
		granted_to: "group:all"
	}
}

job {
	id: "noop-job-v2"
	acl_sets: "default"
	noop: {
		sleep_ms: 1000
	}
}

trigger {
	id: "noop-trigger-v2"
	acl_sets: "default"

	noop: {
		sleep_ms: 1000
		triggers_count: 2
	}

	triggers: "noop-job-v2"
}
`

// project5Cfg should contain validation errors related to only "standard"
// acl_set (referencing AclSet "standard" that does not exist, no OWNER
// AclSet and no READER AclSet).
const project5Cfg = `
acl_sets {
	name: "default"
	acls {
		role: READER
		granted_to: "group:all"
	}
	acls {
		role: OWNER
		granted_to: "group:all"
	}
}

job {
	id: "noop-job-v2"
	acl_sets: "default"
	noop: {
		sleep_ms: 1000
	}
}

trigger {
	id: "noop-trigger-v2"
	acl_sets: "standard"

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
