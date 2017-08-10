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

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"

	"go.chromium.org/luci/scheduler/appengine/acl"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRegisterTaskManagerAndFriends(t *testing.T) {
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
	Convey("validateJobProto works", t, func() {
		c := New("scheduler.cfg").(*catalog)

		call := func(j *messages.Job) error {
			_, err := c.validateJobProto(context.Background(), j)
			return err
		}

		c.RegisterTaskManager(fakeTaskManager{})
		So(call(&messages.Job{}), ShouldErrLike, "missing 'id' field'")
		So(call(&messages.Job{Id: "bad id"}), ShouldErrLike, "not valid value for 'id' field")
		So(call(&messages.Job{
			Id:   "good",
			Task: &messages.TaskDefWrapper{Noop: &messages.NoopTask{}},
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
			msg, err := c.extractTaskProto(&messages.TaskDefWrapper{
				Noop: &messages.NoopTask{},
			})
			So(err, ShouldBeNil)
			So(msg.(*messages.NoopTask), ShouldNotBeNil)

			msg, err = c.extractTaskProto(nil)
			So(err, ShouldErrLike, "expecting a pointer to proto message")
			So(msg, ShouldBeNil)

			msg, err = c.extractTaskProto(&messages.TaskDefWrapper{})
			So(err, ShouldErrLike, "can't find a recognized task definition")
			So(msg, ShouldBeNil)

			msg, err = c.extractTaskProto(&messages.TaskDefWrapper{
				Noop:     &messages.NoopTask{},
				UrlFetch: &messages.UrlFetchTask{},
			})
			So(err, ShouldErrLike, "only one field with task definition must be set")
			So(msg, ShouldBeNil)
		})

		Convey("with Job", func() {
			msg, err := c.extractTaskProto(&messages.Job{
				Id:   "blah",
				Noop: &messages.NoopTask{},
			})
			So(err, ShouldBeNil)
			So(msg.(*messages.NoopTask), ShouldNotBeNil)

			msg, err = c.extractTaskProto(&messages.Job{
				Id: "blah",
			})
			So(err, ShouldErrLike, "can't find a recognized task definition")
			So(msg, ShouldBeNil)

			msg, err = c.extractTaskProto(&messages.Job{
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
		msg, err := c.extractTaskProto(&messages.TaskDefWrapper{
			Noop: &messages.NoopTask{},
		})
		So(err, ShouldErrLike, "boo")
		So(msg, ShouldBeNil)
	})
}

func TestTaskMarshaling(t *testing.T) {
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
		task, err := c.UnmarshalTask(blob)
		So(err, ShouldBeNil)
		So(task, ShouldResemble, &messages.UrlFetchTask{
			Url: "123",
		})

		// Unknown task type.
		_, err = c.marshalTask(&messages.NoopTask{})
		So(err, ShouldErrLike, "unrecognized task definition type *messages.NoopTask")

		// Once registered, but not anymore.
		c = New("scheduler.cfg").(*catalog)
		_, err = c.UnmarshalTask(blob)
		So(err, ShouldErrLike, "can't find a recognized task definition")
	})
}

func TestConfigReading(t *testing.T) {
	Convey("with mocked config", t, func() {
		ctx := testconfig.WithCommonClient(context.Background(), memcfg.New(mockedConfigs))
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
					JobID:    "project1/noop-job-1",
					Acls:     acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:some-admins"}},
					Revision: "847cf9c217f7bad214805f3bea7a30799383940b",
					Schedule: "*/10 * * * * * *",
					Task:     []uint8{0xa, 0x0},
				},
				{
					JobID:    "project1/noop-job-2",
					Acls:     acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:some-admins"}},
					Revision: "847cf9c217f7bad214805f3bea7a30799383940b",
					Schedule: "*/10 * * * * * *",
					Task:     []uint8{0xa, 0x0},
				},
				{
					JobID:    "project1/urlfetch-job-1",
					Acls:     acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:debuggers", "group:some-admins"}},
					Revision: "847cf9c217f7bad214805f3bea7a30799383940b",
					Schedule: "*/10 * * * * * *",
					Task:     []uint8{18, 21, 18, 19, 104, 116, 116, 112, 115, 58, 47, 47, 101, 120, 97, 109, 112, 108, 101, 46, 99, 111, 109},
				},
				{
					JobID:    "project1/urlfetch-job-2",
					Acls:     acl.GrantsByRole{Readers: []string{"group:all"}, Owners: []string{"group:some-admins"}},
					Revision: "847cf9c217f7bad214805f3bea7a30799383940b",
					Schedule: "*/10 * * * * * *",
					Task:     []uint8{18, 21, 18, 19, 104, 116, 116, 112, 115, 58, 47, 47, 101, 120, 97, 109, 112, 108, 101, 46, 99, 111, 109},
				},
			})

			// Make sure URL fetch jobs are parsed correctly and identically.
			newStyleDef := defs[2].Task
			oldStyleDef := defs[3].Task
			So(newStyleDef, ShouldResemble, oldStyleDef)
			msg := messages.TaskDefWrapper{}
			proto.Unmarshal(newStyleDef, &msg)
			So(msg, ShouldResemble, messages.TaskDefWrapper{
				UrlFetch: &messages.UrlFetchTask{
					Url: "https://example.com",
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
		})

		Convey("UnmarshalTask works", func() {
			defs, err := cat.GetProjectJobs(ctx, "project1")
			So(err, ShouldBeNil)

			task, err := cat.UnmarshalTask(defs[0].Task)
			So(err, ShouldBeNil)
			So(task, ShouldResemble, &messages.NoopTask{})

			task, err = cat.UnmarshalTask([]byte("blarg"))
			So(err, ShouldNotBeNil)
			So(task, ShouldBeNil)
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

func (m fakeTaskManager) ValidateProtoMessage(msg proto.Message) error {
	So(msg, ShouldNotBeNil)
	return m.validationErr
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

# Old-style URL fetch job definition.
job {
  id: "urlfetch-job-2"
  schedule: "*/10 * * * * * *"
	acl_sets: "public"

  task: {
    url_fetch: {
      url: "https://example.com"
    }
  }
}

# Will be skipped since SwarmingTask Manager is not registered.
job {
  id: "swarming-job"
  schedule: "*/10 * * * * * *"

  swarming: {}
}
`

const project2Cfg = `
job {
  id: "noop-job-1"
  schedule: "*/10 * * * * * *"
  noop: {}
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
