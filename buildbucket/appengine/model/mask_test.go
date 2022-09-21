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

package model

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto/structmask"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuildMask(t *testing.T) {
	t.Parallel()

	type inner struct {
		FieldOne string `json:"field_one,omitempty"`
		FieldTwo string `json:"field_two,omitempty"`
	}

	type testStruct struct {
		Str  string            `json:"str,omitempty"`
		Int  int               `json:"int,omitempty"`
		Map  map[string]string `json:"map,omitempty"`
		List []inner           `json:"list,omitempty"`
	}

	build := pb.Build{
		Id: 12345,
		Builder: &pb.BuilderID{
			Project: "project",
			Bucket:  "bucket",
			Builder: "builder",
		},
		Number:          7777,
		Canary:          true,
		CreatedBy:       "created_by",
		CanceledBy:      "canceled_by",
		CreateTime:      &timestamppb.Timestamp{Nanos: 11111111},
		StartTime:       &timestamppb.Timestamp{Nanos: 22222222},
		EndTime:         &timestamppb.Timestamp{Nanos: 33333333},
		UpdateTime:      &timestamppb.Timestamp{Nanos: 44444444},
		Status:          pb.Status_SUCCESS,
		SummaryMarkdown: "summary_markdown",
		Critical:        pb.Trinary_YES,
		StatusDetails:   &pb.StatusDetails{Timeout: &pb.StatusDetails_Timeout{}},
		Input: &pb.Build_Input{
			Properties: asStructPb(testStruct{
				Str: "input",
				Int: 123,
				Map: map[string]string{
					"ik1": "iv1",
					"ik2": "iv2",
				},
				List: []inner{
					{"11", "11"},
					{"21", "22"},
				},
			}),
			GerritChanges: []*pb.GerritChange{
				{Host: "h1"},
				{Host: "h2"},
			},
			GitilesCommit: &pb.GitilesCommit{Host: "ihost"},
			Experimental:  true,
		},
		Output: &pb.Build_Output{
			Properties: asStructPb(testStruct{
				Str: "output",
				Int: 123,
				Map: map[string]string{
					"ok1": "ov1",
					"ok2": "ov2",
				},
				List: []inner{
					{"11", "11"},
					{"21", "22"},
				},
			}),
			GitilesCommit: &pb.GitilesCommit{Host: "ohost"},
		},
		Steps: []*pb.Step{
			{Name: "s1", Status: pb.Status_SUCCESS, SummaryMarkdown: "md1"},
			{Name: "s2", Status: pb.Status_SUCCESS, SummaryMarkdown: "md2"},
			{Name: "s3", Status: pb.Status_FAILURE, SummaryMarkdown: "md3"},
		},
		Infra: &pb.BuildInfra{
			Buildbucket: &pb.BuildInfra_Buildbucket{
				Hostname: "bb-host",
				RequestedProperties: asStructPb(testStruct{
					Str: "requested",
					Int: 123,
					Map: map[string]string{
						"rk1": "rv1",
						"rk2": "rv2",
					},
					List: []inner{
						{"11", "11"},
						{"21", "22"},
					},
				}),
			},
			Logdog: &pb.BuildInfra_LogDog{Hostname: "logdog-host"},
		},
		Tags: []*pb.StringPair{
			{Key: "k1", Value: "v1"},
			{Key: "k2", Value: "v2"},
		},
		Exe:               &pb.Executable{},
		SchedulingTimeout: &durationpb.Duration{Seconds: 111},
		ExecutionTimeout:  &durationpb.Duration{Seconds: 222},
		GracePeriod:       &durationpb.Duration{Seconds: 333},
	}

	afterDefaultMask := &pb.Build{
		Builder:    build.Builder,
		Canary:     build.Canary,
		CreateTime: build.CreateTime,
		CreatedBy:  build.CreatedBy,
		Critical:   build.Critical,
		EndTime:    build.EndTime,
		Id:         build.Id,
		Input: &pb.Build_Input{
			Experimental:  build.Input.Experimental,
			GerritChanges: build.Input.GerritChanges,
			GitilesCommit: build.Input.GitilesCommit,
		},
		Number:        build.Number,
		StartTime:     build.StartTime,
		Status:        build.Status,
		StatusDetails: build.StatusDetails,
		UpdateTime:    build.UpdateTime,
	}

	apply := func(m *BuildMask) (*pb.Build, error) {
		b := proto.Clone(&build).(*pb.Build)
		err := m.Trim(b)
		return b, err
	}

	Convey("Default", t, func() {
		Convey("No masks at all", func() {
			m, err := NewBuildMask("", nil, nil)
			So(err, ShouldBeNil)
			b, err := apply(m)
			So(err, ShouldBeNil)
			So(b, ShouldResembleProto, afterDefaultMask)
		})

		Convey("Legacy", func() {
			m, err := NewBuildMask("", &fieldmaskpb.FieldMask{}, nil)
			So(err, ShouldBeNil)
			b, err := apply(m)
			So(err, ShouldBeNil)
			So(b, ShouldResembleProto, afterDefaultMask)
		})

		Convey("BuildMask", func() {
			m, err := NewBuildMask("", nil, &pb.BuildMask{})
			So(err, ShouldBeNil)
			b, err := apply(m)
			So(err, ShouldBeNil)
			So(b, ShouldResembleProto, afterDefaultMask)
		})
	})

	Convey("Legacy", t, func() {
		m, err := NewBuildMask("", &fieldmaskpb.FieldMask{
			Paths: []string{
				"builder",
				"input.properties",
				"steps.*.name", // note: extended syntax
			},
		}, nil)
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)
		So(b, ShouldResembleProto, &pb.Build{
			Builder: build.Builder,
			Input: &pb.Build_Input{
				Properties: build.Input.Properties,
			},
			Steps: []*pb.Step{
				{Name: "s1"},
				{Name: "s2"},
				{Name: "s3"},
			},
		})
	})

	Convey("Simple build mask", t, func() {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{
					"builder",
					"steps",
					"input.properties", // will be returned unfiltered
				},
			},
		})
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)
		So(b, ShouldResembleProto, &pb.Build{
			Builder: build.Builder,
			Steps: []*pb.Step{
				{Name: "s1", Status: pb.Status_SUCCESS, SummaryMarkdown: "md1"},
				{Name: "s2", Status: pb.Status_SUCCESS, SummaryMarkdown: "md2"},
				{Name: "s3", Status: pb.Status_FAILURE, SummaryMarkdown: "md3"},
			},
			Input: &pb.Build_Input{
				Properties: build.Input.Properties,
			},
		})
	})

	Convey("Struct filters", t, func() {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{
					"builder",
					"input.gerrit_changes",
					"output.properties", // will be filtered, since we have a mask below
				},
			},
			InputProperties: []*structmask.StructMask{
				{Path: []string{"str"}},
				{Path: []string{"map", "ik1"}},
				{Path: []string{"list", "*", "field_two"}},
				{Path: []string{"unknown"}},
			},
			OutputProperties: []*structmask.StructMask{
				{Path: []string{"str"}},
			},
			RequestedProperties: []*structmask.StructMask{
				{Path: []string{"unknown"}},
			},
		})
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)
		So(b, ShouldResembleProto, &pb.Build{
			Builder: build.Builder,
			Input: &pb.Build_Input{
				GerritChanges: build.Input.GerritChanges,
				Properties: asStructPb(testStruct{
					Str: "input",
					Map: map[string]string{"ik1": "iv1"},
					List: []inner{
						{"", "11"},
						{"", "22"},
					},
				}),
			},
			Output: &pb.Build_Output{
				Properties: asStructPb(testStruct{
					Str: "output",
				}),
			},
			Infra: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					RequestedProperties: &structpb.Struct{}, // all was filtered out
				},
			},
		})
	})

	Convey("Struct filters and default mask", t, func() {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			OutputProperties: []*structmask.StructMask{
				{Path: []string{"str"}},
			},
		})
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)

		expected := proto.Clone(afterDefaultMask).(*pb.Build)
		expected.Output = &pb.Build_Output{
			Properties: asStructPb(testStruct{Str: "output"}),
		}
		So(b, ShouldResembleProto, expected)
	})

	Convey("Step status with steps", t, func() {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{
					"steps",
				},
			},
			StepStatus: []pb.Status{
				pb.Status_FAILURE,
			},
		})
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)

		expected := &pb.Build{
			Steps: []*pb.Step{
				{
					Name:            "s3",
					Status:          pb.Status_FAILURE,
					SummaryMarkdown: "md3",
				},
			},
		}
		So(b, ShouldResembleProto, expected)
	})

	Convey("Step status no steps", t, func() {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			StepStatus: []pb.Status{
				pb.Status_FAILURE,
			},
		})
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)

		expected := proto.Clone(afterDefaultMask).(*pb.Build)
		expected.Steps = nil
		So(b, ShouldResembleProto, expected)
	})

	Convey("Step status all fields", t, func() {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			AllFields: true,
			StepStatus: []pb.Status{
				pb.Status_FAILURE,
			},
		})
		So(err, ShouldBeNil)

		b, err := apply(m)
		So(err, ShouldBeNil)

		expected := proto.Clone(&build).(*pb.Build)
		expected.Steps = []*pb.Step{
			{
				Name:            "s3",
				Status:          pb.Status_FAILURE,
				SummaryMarkdown: "md3",
			},
		}
		So(b, ShouldResembleProto, expected)
	})

	Convey("Unknown mask paths", t, func() {
		_, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{"builderzzz"},
			},
		})
		So(err, ShouldErrLike, `field "builderzzz" does not exist in message Build`)
	})

	Convey("Bad struct mask", t, func() {
		_, err := NewBuildMask("", nil, &pb.BuildMask{
			InputProperties: []*structmask.StructMask{
				{Path: []string{"'unbalanced"}},
			},
		})
		So(err, ShouldErrLike, `bad "input_properties" struct mask: bad element "'unbalanced" in the mask`)
	})

	Convey("Unsupported extended syntax", t, func() {
		_, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{"steps.*.name"},
			},
		})
		So(err, ShouldErrLike, "no longer supported")
	})

	Convey("Legacy and new at the same time are not allowed", t, func() {
		_, err := NewBuildMask("", &fieldmaskpb.FieldMask{}, &pb.BuildMask{})
		So(err, ShouldErrLike, "can't be used together")
	})

	Convey("all_fields", t, func() {
		Convey("fail", func() {
			_, err := NewBuildMask("", nil, &pb.BuildMask{
				AllFields: true,
				Fields: &fieldmaskpb.FieldMask{
					Paths: []string{"status"},
				},
			})
			So(err, ShouldErrLike, "mask.AllFields is mutually exclusive with other mask fields")
		})
		Convey("pass", func() {
			m, err := NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
			So(err, ShouldBeNil)
			b, err := apply(m)
			So(err, ShouldBeNil)
			So(b, ShouldResembleProto, &build)
		})
	})

	Convey("sanity check mask for list-only permission", t, func() {
		expectedFields := []string{
			"id",
			"status",
			"status_details",
			"can_outlive_parent",
			"ancestor_ids",
		}
		So(BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_LIST_PERMISSION), ShouldResemble, expectedFields)
	})

	Convey("sanity check mask for get-limited permission", t, func() {
		expectedFields := []string{
			"id",
			"builder",
			"number",
			"create_time",
			"start_time",
			"end_time",
			"update_time",
			"cancel_time",
			"status",
			"critical",
			"status_details",
			"input.gitiles_commit",
			"input.gerrit_changes",
			"infra.resultdb",
			"can_outlive_parent",
			"ancestor_ids",
		}
		So(BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_GET_LIMITED_PERMISSION), ShouldResemble, expectedFields)
	})
}

func asStructPb(v interface{}) *structpb.Struct {
	blob, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	s := &structpb.Struct{}
	if err := (protojson.UnmarshalOptions{}).Unmarshal(blob, s); err != nil {
		panic(err)
	}
	return s
}
