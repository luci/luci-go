// Copyright 2016 The LUCI Authors.
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

package target

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCreateTargetFromHostname(t *testing.T) {
	t.Parallel()

	Convey("A target created", t, func() {
		fl := NewFlags()
		fl.SysInfo = &SysInfo{Hostname: "test-c4", Region: "test-region"}

		Convey("for a device with autogenerated hostname should have autogen: hostname prefix", func() {
			fl.TargetType = DeviceType
			fl.AutoGenHostname = true
			fl.SetDefaultsFromHostname()
			target, err := NewFromFlags(&fl)
			So(err, ShouldBeNil)
			So(target, ShouldHaveSameTypeAs, (*NetworkDevice)(nil))
			So(target.(*NetworkDevice).Hostname, ShouldEqual, "autogen:test-c4")
			So(target.(*NetworkDevice).Hostgroup, ShouldEqual, "4")
		})
		Convey("for a device with a static hostname should not have a prefix", func() {
			fl.TargetType = DeviceType
			fl.SetDefaultsFromHostname()
			target, err := NewFromFlags(&fl)
			So(err, ShouldBeNil)
			So(target, ShouldHaveSameTypeAs, (*NetworkDevice)(nil))
			So(target.(*NetworkDevice).Hostname, ShouldEqual, "test-c4")
			So(target.(*NetworkDevice).Hostgroup, ShouldEqual, "4")
		})
		Convey("for a task with autogenerated hostname should have autogen: hostname prefix", func() {
			fl.TargetType = TaskType
			fl.TaskServiceName = "test-service"
			fl.TaskJobName = "test-job"
			fl.AutoGenHostname = true
			fl.SetDefaultsFromHostname()
			target, err := NewFromFlags(&fl)
			So(err, ShouldBeNil)
			So(target, ShouldHaveSameTypeAs, (*Task)(nil))
			So(target.(*Task).HostName, ShouldEqual, "autogen:test-c4")
		})
		Convey("for a task with a static hostname should not have a prefix", func() {
			fl.TargetType = TaskType
			fl.TaskServiceName = "test-service"
			fl.TaskJobName = "test-job"
			fl.SetDefaultsFromHostname()
			target, err := NewFromFlags(&fl)
			So(err, ShouldBeNil)
			So(target, ShouldHaveSameTypeAs, (*Task)(nil))
			So(target.(*Task).HostName, ShouldEqual, "test-c4")
		})
	})
}

func TestTargetContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	deviceTarget := NetworkDevice{
		Metro:     "test-metro",
		Role:      "test-role",
		Hostname:  "test-hostname",
		Hostgroup: "test-hostgroup",
	}
	taskTarget := Task{
		ServiceName: "test-service",
		JobName:     "test-job",
		DataCenter:  "test-datacenter",
		HostName:    "test-hostname",
		TaskNum:     0,
	}

	Convey("Without a target context", t, func() {
		Convey("Get returns nil", func() {
			So(Get(ctx, taskTarget.Type()), ShouldBeNil)
		})
	})
	Convey("With a single target context", t, func() {
		tctx := Set(ctx, &taskTarget)
		Convey("Get returns the target if the type matches", func() {
			So(Get(tctx, taskTarget.Type()), ShouldEqual, &taskTarget)
		})
		Convey("Get returns nil if the type doesn't match", func() {
			So(Get(tctx, deviceTarget.Type()), ShouldBeNil)
		})

		Convey("Get returns the same target object for the same Type", func() {
			clone := (&taskTarget).Clone().(*Task)
			clone.TaskNum += 1

			// Get should return the same target object, whichever target object
			// the type was extracted from, as long as they all are the same type
			// of target instances.
			So(Get(tctx, taskTarget.Type()), ShouldEqual, &taskTarget)
			So(Get(tctx, clone.Type()), ShouldEqual, &taskTarget)
			So(Get(tctx, (*Task)(nil).Type()), ShouldEqual, &taskTarget)
		})
	})

	Convey("With stacked target contexts", t, func() {
		tctx := Set(ctx, &taskTarget)
		tdctx := Set(tctx, &deviceTarget)

		Convey("Child doesn't override the target for different types", func() {
			So(Get(tdctx, taskTarget.Type()), ShouldEqual, &taskTarget)
			So(Get(tdctx, deviceTarget.Type()), ShouldEqual, &deviceTarget)
		})

		Convey("Child overrides the target for the same type", func() {
			clone := (&taskTarget).Clone().(*Task)
			clone.TaskNum += 1

			uptctx := Set(tdctx, clone)
			// The target object for DeviceType should remain the same.
			So(Get(uptctx, deviceTarget.Type()), ShouldEqual, &deviceTarget)
			// but the object for TaskType should have been updated.
			So(Get(uptctx, taskTarget.Type()), ShouldEqual, clone)
			So(Get(uptctx, taskTarget.Type()), ShouldNotEqual, &taskTarget)

			// the parent context should remain the same
			So(Get(tdctx, taskTarget.Type()), ShouldEqual, &taskTarget)
			So(Get(tdctx, deviceTarget.Type()), ShouldEqual, &deviceTarget)
		})
	})
}
