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
	"errors"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"go.chromium.org/luci/common/clock/testclock"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/store"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"

	"go.chromium.org/luci/scheduler/appengine/acl"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRegisterTaskManagerAndFriends(t *testing.T) {
	t.Parallel()

	Convey("RegisterTaskManager works", t, func() {
		c := New("scheduler.cfg")
		So(c.RegisterTaskManager(fakeTaskManager{}), ShouldBeNil)
		So(c.GetTaskManager(&messages.NoopTask{}), ShouldNotBeNil)
		So(c.GetTaskManager(&messages.UrlFetchTask{}), ShouldBeNil)
		So(c.GetTaskManager(nil), ShouldBeNil)
	})

	Convey("RegisterTaskManager bad proto type", t, func() {
		c := New("scheduler.cfg")
		So(c.RegisterTaskManager(brokenTaskManager{}), ShouldErrLike, "expecting pointer to a struct")
	})

	Convey("RegisterTaskManager twice", t, func() {
		c := New("scheduler.cfg")
		So(c.RegisterTaskManager(fakeTaskManager{}), ShouldBeNil)
		So(c.RegisterTaskManager(fakeTaskManager{}), ShouldNotBeNil)
	})
}

func TestProtoValidation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	Convey("validateJobProto works", t, func() {
		c := New("scheduler.cfg").(*catalog)

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
	})

	Convey("extractTaskProto works", t, func() {
		c := New("scheduler.cfg").(*catalog)
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
		c := New("scheduler.cfg").(*catalog)
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
		c := New("scheduler.cfg").(*catalog)
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
		c = New("scheduler.cfg").(*catalog)
		_, err = c.UnmarshalTask(ctx, blob)
		So(err, ShouldErrLike, "can't find a recognized task definition")
	})
}

func TestConfigReading(t *testing.T) {
	t.Parallel()

	Convey("with mocked config", t, func() {
		ctx := testContext()
		ctx = testconfig.WithCommonClient(ctx, memcfg.New(mockedConfigs))
		cat := New("scheduler.cfg")
		cat.RegisterTaskManager(fakeTaskManager{
			name: "noop",
			task: &messages.NoopTask{},
		})
		cat.RegisterTaskManager(fakeTaskManager{
			name: "url_fetch",
			task: &messages.UrlFetchTask{},
		})

		Convey("GetAllProjects works", func() {
			projects, err := cat.GetAllProjects(ctx)
			So(err, ShouldBeNil)
			So(projects, ShouldResemble, []string{"broken", "project1", "project2"})
		})

		Convey("GetProjectJobs works", func() {
			defs, err := cat.GetProjectJobs(ctx, "project1")
			So(err, ShouldBeNil)
			So(defs, ShouldResemble, []Definition{
				{
					JobID:       "project1/noop-job-1",
					Acls:        acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:some-admins"}},
					Revision:    "ab303f699de2d07ab5458e20ea600f3c1adffddb",
					RevisionURL: "https://example.com/view/here/scheduler.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{0xa, 0x0},
				},
				{
					JobID:       "project1/noop-job-2",
					Acls:        acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:some-admins"}},
					Revision:    "ab303f699de2d07ab5458e20ea600f3c1adffddb",
					RevisionURL: "https://example.com/view/here/scheduler.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{0xa, 0x0},
				},
				{
					JobID:       "project1/urlfetch-job-1",
					Acls:        acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:debuggers", "group:some-admins"}},
					Revision:    "ab303f699de2d07ab5458e20ea600f3c1adffddb",
					RevisionURL: "https://example.com/view/here/scheduler.cfg",
					Schedule:    "*/10 * * * * * *",
					Task:        []uint8{18, 21, 18, 19, 104, 116, 116, 112, 115, 58, 47, 47, 101, 120, 97, 109, 112, 108, 101, 46, 99, 111, 109},
				},
			})
			So(getConfiguredProjectValue(ctx, "project1"), ShouldBeTrue)
			So(getConfiguredJobsValue(ctx, "project1", "valid"), ShouldEqual, 3)
			So(getConfiguredJobsValue(ctx, "project1", "invalid"), ShouldEqual, 1)
			So(getConfiguredJobsValue(ctx, "project1", "disabled"), ShouldEqual, 1)
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
	catalog := New("luci-scheduler.cfg")
	catalog.RegisterTaskManager(fakeTaskManager{
		name: "noop",
		task: &messages.NoopTask{},
	})
	catalog.RegisterTaskManager(fakeTaskManager{
		name: "url_fetch",
		task: &messages.UrlFetchTask{},
	})

	Convey("Config validation works", t, func() {
		ctx := &validation.Context{Context: testContext()}
		Convey("correct config file content", func() {
			catalog.ValidateConfig(ctx, "projects/good", "luci-scheduler.cfg", []byte(project3Cfg))
			So(ctx.Finalize(), ShouldBeNil)
		})

		Convey("Config that can't be deserialized", func() {
			catalog.ValidateConfig(ctx, "projects/bad", "luci-scheduler.cfg", []byte("deadbeef"))
			So(ctx.Finalize(), ShouldNotBeNil)
		})

		Convey("semantic errors", func() {
			catalog.ValidateConfig(ctx, "projects/validation-error", "luci-scheduler.cfg", []byte(project4Cfg))
			So(ctx.Finalize(), ShouldErrLike, "referencing AclSet \"standard\" which doesn't exist")
		})
	})
}

func TestConfigPatterns(t *testing.T) {
	Convey("Should return patterns", t, func() {
		catalog := New("luci-scheduler.cfg")
		patterns := catalog.ConfigPatterns(testContext())
		So(len(patterns), ShouldEqual, 1)
		So(patterns[0].ConfigSet.String(), ShouldEqual, "regex:^projects/")
		So(patterns[0].Path.String(), ShouldEqual, "exact:luci-scheduler.cfg")
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

func (m fakeTaskManager) LaunchTask(c context.Context, ctl task.Controller, triggers []*internal.Trigger) error {
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
	c := context.Background()
	c, _ = testclock.UseTime(c, testclock.TestTimeUTC)
	c, _, _ = tsmon.WithFakes(c)
	tsmon.GetState(c).SetStore(store.NewInMemory(&target.Task{}))
	return c
}

func getConfiguredProjectValue(ctx context.Context, project string) bool {
	metricValue, err := tsmon.GetState(ctx).S.Get(ctx, metricConfigValid, time.Time{}, []interface{}{project})
	if err != nil {
		panic(err)
	}
	return metricValue.(bool)
}

func getConfiguredJobsValue(ctx context.Context, project, status string) int64 {
	metricValue, err := tsmon.GetState(ctx).S.Get(ctx, metricConfigJobs, time.Time{}, []interface{}{project, status})
	if err != nil {
		panic(err)
	}
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

  url_fetch: {
    url: "https://example.com"
  }
}

# Will be skipped since BuildbucketTask Manager is not registered.
job {
  id: "buildbucket-job"
  schedule: "*/10 * * * * * *"

  buildbucket: {}
}
`

const project2Cfg = `
job {
  id: "noop-job-1"
  schedule: "*/10 * * * * * *"
  noop: {}
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

// project4cfg should contain validation errors related to only "standard"
// acl_set (referencing AclSet "standard" that does not exist, no OWNER
// AclSet and no READER AclSet).
const project4Cfg = `
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

var mockedConfigs = map[string]memcfg.ConfigSet{
	"projects/project1": {
		"scheduler.cfg": project1Cfg,
	},
	"projects/project2": {
		"scheduler.cfg": project2Cfg,
	},
	"projects/broken": {
		"scheduler.cfg": "broken!!!!111",
	},
}
