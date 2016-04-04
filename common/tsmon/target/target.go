// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package target contains information about the thing that is sending metrics -
// either a NetworkDevice (a machine) or a Task (a service).
// There is a default target that is usually configured with commandline flags
// (flags.go), but a target can also be passed through the Context (context.go)
// if you need to set metric values for a different target.
package target

import (
	"errors"
	"fmt"
	"hash/fnv"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/types"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
)

// A Task is a process or a service running on one or more machine.
type Task pb.Task

// AsProto returns this object as a tsmon.proto.Task message.
func (t *Task) AsProto() *pb.Task { return (*pb.Task)(t) }

// PopulateProto implements Target.
func (t *Task) PopulateProto(d *pb.MetricsData) { d.Task = t.AsProto() }

// IsPopulatedIn returns true if the MetricsData message is for this target.
func (t *Task) IsPopulatedIn(d *pb.MetricsData) bool {
	return d.Task != nil &&
		d.Task.ServiceName == t.AsProto().ServiceName &&
		d.Task.JobName == t.AsProto().JobName &&
		d.Task.DataCenter == t.AsProto().DataCenter &&
		d.Task.HostName == t.AsProto().HostName &&
		d.Task.TaskNum == t.AsProto().TaskNum
}

// Hash returns a uint64 hash of this target.
func (t *Task) Hash() uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%s\n%s\n%s\n%s\n%d",
		t.AsProto().ServiceName,
		t.AsProto().JobName,
		t.AsProto().DataCenter,
		t.AsProto().HostName,
		t.AsProto().TaskNum)
	return h.Sum64()
}

// Clone returns a copy of this object.
func (t *Task) Clone() types.Target {
	return (*Task)(proto.Clone(t.AsProto()).(*pb.Task))
}

// A NetworkDevice is a machine that has a hostname.
type NetworkDevice pb.NetworkDevice

// AsProto returns this object as a tsmon.proto.NetworkDevice message.
func (t *NetworkDevice) AsProto() *pb.NetworkDevice { return (*pb.NetworkDevice)(t) }

// PopulateProto implements Target.
func (t *NetworkDevice) PopulateProto(d *pb.MetricsData) { d.NetworkDevice = t.AsProto() }

// IsPopulatedIn returns true if the MetricsData message is for this target.
func (t *NetworkDevice) IsPopulatedIn(d *pb.MetricsData) bool {
	return d.NetworkDevice != nil &&
		d.NetworkDevice.Alertable == t.AsProto().Alertable &&
		d.NetworkDevice.Realm == t.AsProto().Realm &&
		d.NetworkDevice.Metro == t.AsProto().Metro &&
		d.NetworkDevice.Role == t.AsProto().Role &&
		d.NetworkDevice.Hostname == t.AsProto().Hostname &&
		d.NetworkDevice.Hostgroup == t.AsProto().Hostgroup
}

// Hash returns a uint64 hash of this target.
func (t *NetworkDevice) Hash() uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%t%s\n%s\n%s\n%s\n%s",
		t.AsProto().Alertable,
		t.AsProto().Realm,
		t.AsProto().Metro,
		t.AsProto().Role,
		t.AsProto().Hostname,
		t.AsProto().Hostgroup)
	return h.Sum64()
}

// Clone returns a copy of this object.
func (t *NetworkDevice) Clone() types.Target {
	return (*NetworkDevice)(proto.Clone(t.AsProto()).(*pb.NetworkDevice))
}

// NewFromFlags returns a Target configured from commandline flags.
func NewFromFlags(fl *Flags) (types.Target, error) {
	if fl.TargetType == "task" {
		if fl.TaskServiceName == "" {
			return nil, errors.New(
				"--ts-mon-task-service-name must be provided when using --ts-mon-target-type=task")
		}
		if fl.TaskJobName == "" {
			return nil, errors.New(
				"--ts-mon-task-job-name must be provided when using --ts-mon-target-type=task")
		}

		return (*Task)(&pb.Task{
			ServiceName: fl.TaskServiceName,
			JobName:     fl.TaskJobName,
			DataCenter:  fl.TaskRegion,
			HostName:    fl.TaskHostname,
			TaskNum:     int32(fl.TaskNumber),
		}), nil
	} else if fl.TargetType == "device" {
		return (*NetworkDevice)(&pb.NetworkDevice{
			Alertable: true,
			Realm:     "ACQ_CHROME",
			Metro:     fl.DeviceRegion,
			Role:      fl.DeviceRole,
			Hostname:  fl.DeviceHostname,
			Hostgroup: fl.DeviceNetwork,
		}), nil
	} else {
		return nil, fmt.Errorf("unknown --ts-mon-target-type '%s'", fl.TargetType)
	}
}
