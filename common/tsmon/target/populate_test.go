// Copyright 2021 The LUCI Authors.
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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"

	"google.golang.org/protobuf/proto"
)

func TestTargetPopulate(t *testing.T) {
	t.Parallel()
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
		TaskNum:     1,
	}

	ftt.Run("Populate metrics collection", t, func(t *ftt.Test) {
		t.Run("With DeviceTarget", func(t *ftt.Test) {
			d := &pb.MetricsCollection{}
			deviceTarget.PopulateProto(d)
			assert.Loosely(t, d.RootLabels, should.Resemble([]*pb.MetricsCollection_RootLabels{
				{
					Key:   proto.String("proxy_environment"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "pa"},
				},
				{
					Key:   proto.String("acquisition_name"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "mon-chrome-infra"},
				},
				{
					Key:   proto.String("proxy_zone"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "atl"},
				},
				{
					Key:   proto.String("pop"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: ""},
				},
				{
					Key:   proto.String("alertable"),
					Value: &pb.MetricsCollection_RootLabels_BoolValue{BoolValue: true},
				},
				{
					Key:   proto.String("realm"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "ACQ_CHROME"},
				},
				{
					Key:   proto.String("asn"),
					Value: &pb.MetricsCollection_RootLabels_Int64Value{Int64Value: 0},
				},
				{
					Key:   proto.String("metro"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-metro"},
				},
				{
					Key:   proto.String("role"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-role"},
				},
				{
					Key:   proto.String("hostname"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-hostname"},
				},
				{
					Key:   proto.String("vendor"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: ""},
				},
				{
					Key:   proto.String("hostgroup"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-hostgroup"},
				},
			}))
		})

		t.Run("With TaskTarget", func(t *ftt.Test) {
			d := &pb.MetricsCollection{}
			taskTarget.PopulateProto(d)
			assert.Loosely(t, d.RootLabels, should.Resemble([]*pb.MetricsCollection_RootLabels{
				{
					Key:   proto.String("proxy_environment"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "pa"},
				},
				{
					Key:   proto.String("acquisition_name"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "mon-chrome-infra"},
				},
				{
					Key:   proto.String("proxy_zone"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "atl"},
				},
				{
					Key:   proto.String("service_name"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-service"},
				},
				{
					Key:   proto.String("job_name"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-job"},
				},
				{
					Key:   proto.String("data_center"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-datacenter"},
				},
				{
					Key:   proto.String("host_name"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-hostname"},
				},
				{
					Key:   proto.String("task_num"),
					Value: &pb.MetricsCollection_RootLabels_Int64Value{Int64Value: 1},
				},
			}))
		})
	})
}
