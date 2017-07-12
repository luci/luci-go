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
