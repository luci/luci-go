// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package target

import (
	"github.com/golang/protobuf/proto"

	pb "github.com/luci/luci-go/common/tsmon/ts_mon_proto"
)

// PopulateProto implements Target.
func (t *Task) PopulateProto(d *pb.MetricsCollection) {
	d.TargetSchema = &pb.MetricsCollection_Task{
		&pb.Task{
			ServiceName: &t.ServiceName,
			JobName:     &t.JobName,
			DataCenter:  &t.DataCenter,
			HostName:    &t.HostName,
			TaskNum:     &t.TaskNum,
		},
	}
}

// PopulateProto implements Target.
func (t *NetworkDevice) PopulateProto(d *pb.MetricsCollection) {
	d.TargetSchema = &pb.MetricsCollection_NetworkDevice{
		&pb.NetworkDevice{
			Alertable: proto.Bool(true),
			Realm:     proto.String("ACQ_CHROME"),
			Metro:     &t.Metro,
			Role:      &t.Role,
			Hostname:  &t.Hostname,
			Hostgroup: &t.Hostgroup,
		},
	}
}
