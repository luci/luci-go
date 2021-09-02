package target

import (
	"testing"

	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"

	"google.golang.org/protobuf/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("Populate metrics collection", t, func() {
		Convey("With DeviceTarget", func() {
			d := &pb.MetricsCollection{
				RootLabels: []*pb.MetricsCollection_RootLabels{
					{
						Key:   proto.String("proxy_zone"),
						Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "abc"},
					},
				},
			}

			deviceTarget.PopulateProto(d)

			So(d.TargetSchema, ShouldBeNil)
			So(d.RootLabels, ShouldResembleProto, []*pb.MetricsCollection_RootLabels{
				{
					Key:   proto.String("proxy_zone"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "abc"},
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
					Key:   proto.String("hostgroup"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "test-hostgroup"},
				},
			})
		})

		Convey("With TaskTarget", func() {
			d := &pb.MetricsCollection{
				RootLabels: []*pb.MetricsCollection_RootLabels{
					{
						Key:   proto.String("proxy_zone"),
						Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "abc"},
					},
				},
			}

			taskTarget.PopulateProto(d)

			So(d.TargetSchema, ShouldBeNil)
			So(d.RootLabels, ShouldResembleProto, []*pb.MetricsCollection_RootLabels{
				{
					Key:   proto.String("proxy_zone"),
					Value: &pb.MetricsCollection_RootLabels_StringValue{StringValue: "abc"},
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
			})
		})
	})
}
