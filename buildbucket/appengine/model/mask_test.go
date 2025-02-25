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

	"go.chromium.org/luci/common/proto/structmask"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
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

	ftt.Run("Default", t, func(t *ftt.Test) {
		t.Run("No masks at all", func(t *ftt.Test) {
			m, err := NewBuildMask("", nil, nil)
			assert.Loosely(t, err, should.BeNil)
			b, err := apply(m)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b, should.Match(afterDefaultMask))
		})

		t.Run("Legacy", func(t *ftt.Test) {
			m, err := NewBuildMask("", &fieldmaskpb.FieldMask{}, nil)
			assert.Loosely(t, err, should.BeNil)
			b, err := apply(m)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b, should.Match(afterDefaultMask))
		})

		t.Run("BuildMask", func(t *ftt.Test) {
			m, err := NewBuildMask("", nil, &pb.BuildMask{})
			assert.Loosely(t, err, should.BeNil)
			b, err := apply(m)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b, should.Match(afterDefaultMask))
		})
	})

	ftt.Run("Legacy", t, func(t *ftt.Test) {
		m, err := NewBuildMask("", &fieldmaskpb.FieldMask{
			Paths: []string{
				"builder",
				"input.properties",
				"steps.*.name", // note: extended syntax
			},
		}, nil)
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.Match(&pb.Build{
			Builder: build.Builder,
			Input: &pb.Build_Input{
				Properties: build.Input.Properties,
			},
			Steps: []*pb.Step{
				{Name: "s1"},
				{Name: "s2"},
				{Name: "s3"},
			},
		}))
	})

	ftt.Run("Simple build mask", t, func(t *ftt.Test) {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{
					"builder",
					"steps",
					"input.properties", // will be returned unfiltered
				},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.Match(&pb.Build{
			Builder: build.Builder,
			Steps: []*pb.Step{
				{Name: "s1", Status: pb.Status_SUCCESS, SummaryMarkdown: "md1"},
				{Name: "s2", Status: pb.Status_SUCCESS, SummaryMarkdown: "md2"},
				{Name: "s3", Status: pb.Status_FAILURE, SummaryMarkdown: "md3"},
			},
			Input: &pb.Build_Input{
				Properties: build.Input.Properties,
			},
		}))
	})

	ftt.Run("Struct filters", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, b, should.Match(&pb.Build{
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
		}))
	})

	ftt.Run("Struct filters and default mask", t, func(t *ftt.Test) {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			OutputProperties: []*structmask.StructMask{
				{Path: []string{"str"}},
			},
		})
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)

		expected := proto.Clone(afterDefaultMask).(*pb.Build)
		expected.Output = &pb.Build_Output{
			Properties: asStructPb(testStruct{Str: "output"}),
		}
		assert.Loosely(t, b, should.Match(expected))
	})

	ftt.Run("Step status with steps", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)

		expected := &pb.Build{
			Steps: []*pb.Step{
				{
					Name:            "s3",
					Status:          pb.Status_FAILURE,
					SummaryMarkdown: "md3",
				},
			},
		}
		assert.Loosely(t, b, should.Match(expected))
	})

	ftt.Run("Step status no steps", t, func(t *ftt.Test) {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			StepStatus: []pb.Status{
				pb.Status_FAILURE,
			},
		})
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)

		expected := proto.Clone(afterDefaultMask).(*pb.Build)
		expected.Steps = nil
		assert.Loosely(t, b, should.Match(expected))
	})

	ftt.Run("Step status all fields", t, func(t *ftt.Test) {
		m, err := NewBuildMask("", nil, &pb.BuildMask{
			AllFields: true,
			StepStatus: []pb.Status{
				pb.Status_FAILURE,
			},
		})
		assert.Loosely(t, err, should.BeNil)

		b, err := apply(m)
		assert.Loosely(t, err, should.BeNil)

		expected := proto.Clone(&build).(*pb.Build)
		expected.Steps = []*pb.Step{
			{
				Name:            "s3",
				Status:          pb.Status_FAILURE,
				SummaryMarkdown: "md3",
			},
		}
		assert.Loosely(t, b, should.Match(expected))
	})

	ftt.Run("Unknown mask paths", t, func(t *ftt.Test) {
		_, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{"builderzzz"},
			},
		})
		assert.Loosely(t, err, should.ErrLike(`field "builderzzz" does not exist in message Build`))
	})

	ftt.Run("Bad struct mask", t, func(t *ftt.Test) {
		_, err := NewBuildMask("", nil, &pb.BuildMask{
			InputProperties: []*structmask.StructMask{
				{Path: []string{"'unbalanced"}},
			},
		})
		assert.Loosely(t, err, should.ErrLike(`bad "input_properties" struct mask: bad element "'unbalanced" in the mask`))
	})

	ftt.Run("Unsupported extended syntax", t, func(t *ftt.Test) {
		_, err := NewBuildMask("", nil, &pb.BuildMask{
			Fields: &fieldmaskpb.FieldMask{
				Paths: []string{"steps.*.name"},
			},
		})
		assert.Loosely(t, err, should.ErrLike("no longer supported"))
	})

	ftt.Run("Legacy and new at the same time are not allowed", t, func(t *ftt.Test) {
		_, err := NewBuildMask("", &fieldmaskpb.FieldMask{}, &pb.BuildMask{})
		assert.Loosely(t, err, should.ErrLike("can't be used together"))
	})

	ftt.Run("all_fields", t, func(t *ftt.Test) {
		t.Run("fail", func(t *ftt.Test) {
			_, err := NewBuildMask("", nil, &pb.BuildMask{
				AllFields: true,
				Fields: &fieldmaskpb.FieldMask{
					Paths: []string{"status"},
				},
			})
			assert.Loosely(t, err, should.ErrLike("mask.AllFields is mutually exclusive with other mask fields"))
		})
		t.Run("pass", func(t *ftt.Test) {
			m, err := NewBuildMask("", nil, &pb.BuildMask{AllFields: true})
			assert.Loosely(t, err, should.BeNil)
			b, err := apply(m)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b, should.Match(&build))
		})
	})

	ftt.Run("sanity check mask for list-only permission", t, func(t *ftt.Test) {
		expectedFields := []string{
			"id",
			"status",
			"status_details",
			"can_outlive_parent",
			"ancestor_ids",
		}
		assert.Loosely(t, BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_LIST_PERMISSION), should.Match(expectedFields))
	})

	ftt.Run("sanity check mask for get-limited permission", t, func(t *ftt.Test) {
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
		assert.Loosely(t, BuildFieldsWithVisibility(pb.BuildFieldVisibility_BUILDS_GET_LIMITED_PERMISSION), should.Match(expectedFields))
	})
}

func asStructPb(v any) *structpb.Struct {
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
