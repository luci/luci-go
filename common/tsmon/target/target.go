// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package target contains information about the thing that is sending metrics -
// either a NetworkDevice (a machine) or a Task (a service).
package target

import (
	"errors"
	"fmt"

	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
)

// A Target knows how to put information about itself in a MetricsData message.
type Target interface {
	PopulateProto(d *pb.MetricsData)
}

// A Task is a process or a service running on one or more machine.
type Task pb.Task

// AsProto returns this object as a tsmon.proto.Task message.
func (t *Task) AsProto() *pb.Task { return (*pb.Task)(t) }

// PopulateProto implements Target.
func (t *Task) PopulateProto(d *pb.MetricsData) { d.Task = t.AsProto() }

// A NetworkDevice is a machine that has a hostname.
type NetworkDevice pb.NetworkDevice

// AsProto returns this object as a tsmon.proto.NetworkDevice message.
func (t *NetworkDevice) AsProto() *pb.NetworkDevice { return (*pb.NetworkDevice)(t) }

// PopulateProto implements Target.
func (t *NetworkDevice) PopulateProto(d *pb.MetricsData) { d.NetworkDevice = t.AsProto() }

// NewFromFlags returns a Target configured from commandline flags.
func NewFromFlags(fl *Flags) (Target, error) {
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
			ServiceName: proto.String(fl.TaskServiceName),
			JobName:     proto.String(fl.TaskJobName),
			DataCenter:  proto.String(fl.TaskRegion),
			HostName:    proto.String(fl.TaskHostname),
			TaskNum:     proto.Int32(int32(fl.TaskNumber)),
		}), nil
	} else if fl.TargetType == "device" {
		return (*NetworkDevice)(&pb.NetworkDevice{
			Alertable: proto.Bool(true),
			Realm:     proto.String("ACQ_CHROME"),
			Metro:     proto.String(fl.DeviceRegion),
			Role:      proto.String(fl.DeviceRole),
			Hostname:  proto.String(fl.DeviceHostname),
			Hostgroup: proto.String(fl.DeviceNetwork),
		}), nil
	} else {
		return nil, fmt.Errorf("unknown --ts-mon-target-type '%s'", fl.TargetType)
	}
}
