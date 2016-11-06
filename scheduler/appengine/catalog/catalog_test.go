// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package catalog

import (
	"errors"
	"testing"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"
	"google.golang.org/api/pubsub/v1"

	"github.com/luci/luci-go/common/config"
	memcfg "github.com/luci/luci-go/common/config/impl/memory"

	"github.com/luci/luci-go/scheduler/appengine/messages"
	"github.com/luci/luci-go/scheduler/appengine/task"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestRegisterTaskManagerAndFriends(t *testing.T) {
	Convey("RegisterTaskManager works", t, func() {
		c := New("scheduler.cfg")
		So(c.RegisterTaskManager(noopTaskManager{}), ShouldBeNil)
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
		So(c.RegisterTaskManager(noopTaskManager{}), ShouldBeNil)
		So(c.RegisterTaskManager(noopTaskManager{}), ShouldNotBeNil)
	})
}

func TestProtoValidation(t *testing.T) {
	Convey("validateJobProto works", t, func() {
		c := New("scheduler.cfg").(*catalog)
		c.RegisterTaskManager(noopTaskManager{})
		So(c.validateJobProto(nil), ShouldErrLike, "job must be specified")
		So(c.validateJobProto(&messages.Job{}), ShouldErrLike, "missing 'id' field'")
		So(c.validateJobProto(&messages.Job{Id: "bad id"}), ShouldErrLike, "not valid value for 'id' field")
		So(c.validateJobProto(&messages.Job{Id: "good"}), ShouldErrLike, "missing 'schedule' field")
		So(c.validateJobProto(&messages.Job{
			Id:       "good",
			Schedule: "blah",
		}), ShouldErrLike, "not valid value for 'schedule' field")
		So(c.validateJobProto(&messages.Job{
			Id:       "good",
			Schedule: "* * * * *",
		}), ShouldErrLike, "missing 'task' field")
		So(c.validateJobProto(&messages.Job{
			Id:       "good",
			Schedule: "* * * * *",
			Task:     &messages.Task{Noop: &messages.NoopTask{}},
		}), ShouldBeNil)
	})

	Convey("extractTaskProto works", t, func() {
		c := New("scheduler.cfg").(*catalog)

		msg, err := c.extractTaskProto(nil)
		So(err, ShouldErrLike, "missing 'task' field")
		So(msg, ShouldBeNil)

		msg, err = c.extractTaskProto(&messages.Task{})
		So(err, ShouldErrLike, "at least one field must be set")
		So(msg, ShouldBeNil)

		msg, err = c.extractTaskProto(&messages.Task{
			Noop:     &messages.NoopTask{},
			UrlFetch: &messages.UrlFetchTask{},
		})
		So(err, ShouldErrLike, "only one field must be set")
		So(msg, ShouldBeNil)

		msg, err = c.extractTaskProto(&messages.Task{Noop: &messages.NoopTask{}})
		So(err, ShouldErrLike, "unknown task type")
		So(msg, ShouldBeNil)

		c = New("scheduler.cfg").(*catalog)
		c.RegisterTaskManager(noopTaskManager{errors.New("boo")})
		msg, err = c.extractTaskProto(&messages.Task{Noop: &messages.NoopTask{}})
		So(err, ShouldErrLike, "boo")
		So(msg, ShouldBeNil)

		c = New("scheduler.cfg").(*catalog)
		c.RegisterTaskManager(noopTaskManager{})
		msg, err = c.extractTaskProto(&messages.Task{Noop: &messages.NoopTask{}})
		So(err, ShouldBeNil)
		So(msg.(*messages.NoopTask), ShouldNotBeNil)
	})
}

func TestConfigReading(t *testing.T) {
	Convey("with mocked config", t, func() {
		ctx := config.SetImplementation(context.Background(), memcfg.New(mockedConfigs))
		cat := New("scheduler.cfg")
		cat.RegisterTaskManager(noopTaskManager{})

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
					Revision: "4065e915c1d0ce93ff98b1741f202927d81157f6",
					Schedule: "*/10 * * * * * *",
					Task:     []uint8{0xa, 0x0},
				},
				{
					JobID:    "project1/noop-job-2",
					Revision: "4065e915c1d0ce93ff98b1741f202927d81157f6",
					Schedule: "*/10 * * * * * *",
					Task:     []uint8{0xa, 0x0},
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

type noopTaskManager struct {
	validationErr error
}

func (m noopTaskManager) Name() string {
	return "testing"
}

func (m noopTaskManager) ProtoMessageType() proto.Message {
	return &messages.NoopTask{}
}

func (m noopTaskManager) ValidateProtoMessage(msg proto.Message) error {
	// Let it panic on a wrong type.
	So(msg.(*messages.NoopTask), ShouldNotBeNil)
	return m.validationErr
}

func (m noopTaskManager) LaunchTask(c context.Context, ctl task.Controller) error {
	// Let it panic on a wrong type.
	So(ctl.Task().(*messages.NoopTask), ShouldNotBeNil)
	return nil
}

func (m noopTaskManager) AbortTask(c context.Context, ctl task.Controller) error {
	return nil
}

func (m noopTaskManager) HandleNotification(c context.Context, ctl task.Controller, msg *pubsub.PubsubMessage) error {
	return errors.New("not implemented")
}

func (m noopTaskManager) HandleTimer(c context.Context, ctl task.Controller, name string, payload []byte) error {
	return errors.New("not implemented")
}

type brokenTaskManager struct {
	noopTaskManager
}

func (b brokenTaskManager) ProtoMessageType() proto.Message {
	return nil
}

////

const project1Cfg = `
job {
  id: "noop-job-1"
  schedule: "*/10 * * * * * *"
  task: {
    noop: {}
  }
}

job {
  id: "noop-job-2"
  schedule: "*/10 * * * * * *"
  task: {
    noop: {}
  }
}

job {
  id: "noop-job-3"
  schedule: "*/10 * * * * * *"
  disabled: true
  task: {
    noop: {}
  }
}

# Will be skipped since UrlFetchTask Manager is not registered.
job {
  id: "noop-job-4"
  schedule: "*/10 * * * * * *"
  task: {
    url_fetch: {}
  }
}
`

const project2Cfg = `
job {
  id: "noop-job-1"
  schedule: "*/10 * * * * * *"
  task: {
    noop: {}
  }
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
