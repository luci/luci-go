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
	"reflect"

	"google.golang.org/protobuf/proto"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
)

// PopulateProto implements Target.
func (t *Task) PopulateProto(d *pb.MetricsCollection) {
	d.RootLabels = append(
		d.RootLabels,
		// ProdX automatically adds and sets the following fields
		// with own values, even if tsmon doesn't add them in RootLabels,
		// except that proxy_zone may be required under certain conditions.
		//
		// This adds them to avoid unexpected data loss,
		// just in case the above assumptions change.
		RootLabel("proxy_environment", "pa"),
		RootLabel("acquisition_name", "mon-chrome-infra"),
		RootLabel("proxy_zone", "atl"),

		RootLabel("service_name", t.ServiceName),
		RootLabel("job_name", t.JobName),
		RootLabel("data_center", t.DataCenter),
		RootLabel("host_name", t.HostName),
		RootLabel("task_num", int64(t.TaskNum)),
	)
}

// PopulateProto implements Target.
func (t *NetworkDevice) PopulateProto(d *pb.MetricsCollection) {
	d.RootLabels = append(
		d.RootLabels,
		// ProdX automatically adds and sets the following fields
		// with own values. Even if tsmon sets them with own values,
		// the custom values are ignored.
		//
		// This adds them to avoid unexpected data loss,
		// just in case the above assumptions change.
		RootLabel("proxy_environment", "pa"),
		RootLabel("acquisition_name", "mon-chrome-infra"),
		RootLabel("proxy_zone", "atl"),

		RootLabel("pop", ""),
		RootLabel("alertable", true),
		RootLabel("realm", "ACQ_CHROME"),
		RootLabel("asn", int64(0)),
		RootLabel("metro", t.Metro),
		RootLabel("role", t.Role),
		RootLabel("hostname", t.Hostname),
		RootLabel("vendor", ""),
		RootLabel("hostgroup", t.Hostgroup),
	)
}

func RootLabel(key string, value interface{}) *pb.MetricsCollection_RootLabels {
	label := &pb.MetricsCollection_RootLabels{Key: proto.String(key)}

	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.String:
		label.Value = &pb.MetricsCollection_RootLabels_StringValue{
			StringValue: value.(string),
		}
	case reflect.Int64:
		label.Value = &pb.MetricsCollection_RootLabels_Int64Value{
			Int64Value: value.(int64),
		}
	case reflect.Bool:
		label.Value = &pb.MetricsCollection_RootLabels_BoolValue{
			BoolValue: value.(bool),
		}
	default:
		panic("unsupported type; all target fields must be one of string, int64, or bool.")
	}
	return label
}
