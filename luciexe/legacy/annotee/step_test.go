// Copyright 2018 The LUCI Authors.
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

package annotee

import (
	"context"
	"fmt"
	"net/url"
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "go.chromium.org/luci/buildbucket/proto"
	annotpb "go.chromium.org/luci/luciexe/legacy/annotee/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var annotationStep = &annotpb.Step{
	Substep: asSubSteps(
		&annotpb.Step{
			Name:   "running step",
			Status: annotpb.Status_RUNNING,
		},
		&annotpb.Step{
			Name:    "successful step",
			Status:  annotpb.Status_SUCCESS,
			Started: &timestamppb.Timestamp{Seconds: 1400000000},
			Ended:   &timestamppb.Timestamp{Seconds: 1400001000},
		},
		&annotpb.Step{
			Name:    "failed step",
			Status:  annotpb.Status_FAILURE,
			Started: &timestamppb.Timestamp{Seconds: 1400000000},
			Ended:   &timestamppb.Timestamp{Seconds: 1400001000},
		},
		&annotpb.Step{
			Name:           "infra-failed step",
			Status:         annotpb.Status_FAILURE,
			FailureDetails: &annotpb.FailureDetails{Type: annotpb.FailureDetails_EXCEPTION},
			Started:        &timestamppb.Timestamp{Seconds: 1400000000},
			Ended:          &timestamppb.Timestamp{Seconds: 1400001000},
		},
		&annotpb.Step{
			Name:           "with failure details text",
			Status:         annotpb.Status_FAILURE,
			FailureDetails: &annotpb.FailureDetails{Text: "failure_details_text"},
			Started:        &timestamppb.Timestamp{Seconds: 1400000000},
			Ended:          &timestamppb.Timestamp{Seconds: 1400001000},
		},
		&annotpb.Step{
			Name: "with text",
			Text: []string{"text1", "text2"},
		},
		&annotpb.Step{
			Name:         "with stdio",
			StdoutStream: &annotpb.LogdogStream{Name: "steps/setup_build/0/stdout"},
			StderrStream: &annotpb.LogdogStream{Name: "steps/setup_build/0/stderr"},
		},
		&annotpb.Step{
			Name: "other links",
			OtherLinks: []*annotpb.AnnotationLink{
				{
					Label: "logdog link",
					Value: &annotpb.AnnotationLink_LogdogStream{
						LogdogStream: &annotpb.LogdogStream{Name: "steps/setup_build/0/logs/run_recipe/0"},
					},
				},
				{
					Label: "1",
					Value: &annotpb.AnnotationLink_Url{
						Url: "https://example.com/1(foo)",
					},
				},
				{
					Label: "with-ampersand",
					Value: &annotpb.AnnotationLink_Url{
						Url: "https://example.com?a=1&timestamp=2",
					},
				},
			},
		},
		&annotpb.Step{
			Name: "substeps",
			// This time will be overridden by children.
			Started: &timestamppb.Timestamp{Seconds: 1500000500},
			Ended:   &timestamppb.Timestamp{Seconds: 1500000501},
			Substep: asSubSteps(
				&annotpb.Step{
					Name:   "child",
					Status: annotpb.Status_SUCCESS,
					Substep: asSubSteps(
						&annotpb.Step{
							Name:    "descendant0",
							Status:  annotpb.Status_FAILURE,
							Started: &timestamppb.Timestamp{Seconds: 1500000000},
							Ended:   &timestamppb.Timestamp{Seconds: 1500001000},
						},
						&annotpb.Step{
							Name:           "descendant1",
							Status:         annotpb.Status_FAILURE,
							FailureDetails: &annotpb.FailureDetails{Type: annotpb.FailureDetails_EXCEPTION},
							Started:        &timestamppb.Timestamp{Seconds: 1500001000},
							Ended:          &timestamppb.Timestamp{Seconds: 1500002000},
						},
					),
				},
				&annotpb.Step{
					Name:    "child2",
					Status:  annotpb.Status_SUCCESS,
					Started: &timestamppb.Timestamp{Seconds: 1500002000},
					Ended:   &timestamppb.Timestamp{Seconds: 1500003000},
				},
				&annotpb.Step{
					Name:    "child3_unfinished",
					Status:  annotpb.Status_RUNNING,
					Started: &timestamppb.Timestamp{Seconds: 1500003000},
				},
			),
		},
		&annotpb.Step{
			Name: "started_parent",
			Substep: asSubSteps(
				&annotpb.Step{
					Name:    "descendant",
					Status:  annotpb.Status_RUNNING,
					Started: &timestamppb.Timestamp{Seconds: 1500000000},
				},
			),
		},
		&annotpb.Step{
			Name:         "duplicate_log_name",
			StdoutStream: &annotpb.LogdogStream{Name: "steps/duplicate_log_name/0/stdout"},
			StderrStream: &annotpb.LogdogStream{Name: "steps/duplicate_log_name/0/stderr"},
			OtherLinks: []*annotpb.AnnotationLink{
				{
					Label: "stdout",
					Value: &annotpb.AnnotationLink_LogdogStream{
						LogdogStream: &annotpb.LogdogStream{Name: "steps/duplicate_log_name/0/stdout"},
					},
				},
			},
		},
		&annotpb.Step{Name: "dup step name"},
		&annotpb.Step{Name: "dup step name"},
		&annotpb.Step{
			Name: "parent_prefix",
			Substep: asSubSteps(
				&annotpb.Step{
					Name: "parent_prefix.child",
					Substep: asSubSteps(
						&annotpb.Step{
							Name: "parent_prefix.child.grandchild",
						},
					),
				},
			),
		},
		&annotpb.Step{
			Name:    "start time is a bit greater than end time",
			Status:  annotpb.Status_SUCCESS,
			Started: &timestamppb.Timestamp{Seconds: 1500000000, Nanos: 2},
			Ended:   &timestamppb.Timestamp{Seconds: 1500000000, Nanos: 1},
		},
	),
}

type calcURLFunc func(logName string) string

var expectedStepsFn = func(urlFunc, viewerURLFunc calcURLFunc) []*pb.Step {
	return []*pb.Step{
		{
			Name:   "running step",
			Status: pb.Status_SCHEDULED,
		},
		{
			Name:      "successful step",
			Status:    pb.Status_SUCCESS,
			StartTime: &timestamppb.Timestamp{Seconds: 1400000000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1400001000},
		},
		{
			Name:      "failed step",
			Status:    pb.Status_FAILURE,
			StartTime: &timestamppb.Timestamp{Seconds: 1400000000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1400001000},
		},
		{
			Name:      "infra-failed step",
			Status:    pb.Status_INFRA_FAILURE,
			StartTime: &timestamppb.Timestamp{Seconds: 1400000000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1400001000},
		},
		{
			Name:            "with failure details text",
			Status:          pb.Status_FAILURE,
			SummaryMarkdown: "failure_details_text",
			StartTime:       &timestamppb.Timestamp{Seconds: 1400000000},
			EndTime:         &timestamppb.Timestamp{Seconds: 1400001000},
		},
		{
			Name:            "with text",
			Status:          pb.Status_SCHEDULED,
			SummaryMarkdown: "\n\n<div>text1 text2</div>\n\n",
		},
		{
			Name:   "with stdio",
			Status: pb.Status_SCHEDULED,
			Logs: []*pb.Log{
				{
					Name:    "stdout",
					Url:     urlFunc("steps/setup_build/0/stdout"),
					ViewUrl: viewerURLFunc("steps/setup_build/0/stdout"),
				},
				{
					Name:    "stderr",
					Url:     urlFunc("steps/setup_build/0/stderr"),
					ViewUrl: viewerURLFunc("steps/setup_build/0/stderr"),
				},
			},
		},
		{
			Name:            "other links",
			Status:          pb.Status_SCHEDULED,
			SummaryMarkdown: "* [1](https://example.com/1\\(foo\\))\n* [with-ampersand](https://example.com?a=1&amp;timestamp=2)",
			Logs: []*pb.Log{
				{
					Name:    "logdog link",
					Url:     urlFunc("steps/setup_build/0/logs/run_recipe/0"),
					ViewUrl: viewerURLFunc("steps/setup_build/0/logs/run_recipe/0"),
				},
			},
		},
		{
			Name:      "substeps",
			Status:    pb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 1500000000},
		},
		{
			Name:      "substeps|child",
			Status:    pb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 1500000000},
		},
		{
			Name:      "substeps|child|descendant0",
			Status:    pb.Status_FAILURE,
			StartTime: &timestamppb.Timestamp{Seconds: 1500000000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1500001000},
		},
		{
			Name:      "substeps|child|descendant1",
			Status:    pb.Status_INFRA_FAILURE,
			StartTime: &timestamppb.Timestamp{Seconds: 1500001000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1500002000},
		},
		{
			Name:      "substeps|child2",
			Status:    pb.Status_SUCCESS,
			StartTime: &timestamppb.Timestamp{Seconds: 1500002000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1500003000},
		},
		{
			Name:      "substeps|child3_unfinished",
			Status:    pb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 1500003000},
		},
		{
			Name:      "started_parent",
			Status:    pb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 1500000000},
		},
		{
			Name:      "started_parent|descendant",
			Status:    pb.Status_STARTED,
			StartTime: &timestamppb.Timestamp{Seconds: 1500000000},
		},
		{
			Name:   "duplicate_log_name",
			Status: pb.Status_SCHEDULED,
			Logs: []*pb.Log{
				{
					Name:    "stdout",
					Url:     urlFunc("steps/duplicate_log_name/0/stdout"),
					ViewUrl: viewerURLFunc("steps/duplicate_log_name/0/stdout"),
				},
				{
					Name:    "stderr",
					Url:     urlFunc("steps/duplicate_log_name/0/stderr"),
					ViewUrl: viewerURLFunc("steps/duplicate_log_name/0/stderr"),
				},
			},
		},
		{
			Name:   "dup step name",
			Status: pb.Status_SCHEDULED,
		},
		{
			Name:   "dup step name (2)",
			Status: pb.Status_SCHEDULED,
		},
		{
			Name:   "parent_prefix",
			Status: pb.Status_SCHEDULED,
		},
		{
			Name:   "parent_prefix|child",
			Status: pb.Status_SCHEDULED,
		},
		{
			Name:   "parent_prefix|child|grandchild",
			Status: pb.Status_SCHEDULED,
		},
		{
			Name:      "start time is a bit greater than end time",
			Status:    pb.Status_SUCCESS,
			StartTime: &timestamppb.Timestamp{Seconds: 1500000000, Nanos: 1},
			EndTime:   &timestamppb.Timestamp{Seconds: 1500000000, Nanos: 2},
		},
	}

}

func TestConvertBuildStep(t *testing.T) {
	t.Parallel()

	Convey("convert", t, func() {
		Convey("with LogDog URL constructed", func() {
			host := "logdog.example.com"
			prefix := "project/prefix"

			actual, err := ConvertBuildSteps(context.Background(), annotationStep.Substep, true, host, prefix)
			So(err, ShouldBeNil)
			expected := expectedStepsFn(
				func(logName string) string {
					return fmt.Sprintf("logdog://%s/%s/+/%s", host, prefix, logName)
				},
				func(logName string) string {
					return fmt.Sprintf("https://%s/v/?s=%s", host, url.QueryEscape(prefix+"/+/"+logName))
				},
			)
			So(actual, ShouldResembleProto, expected)
		})
		Convey("without LogDog URL constructed", func() {
			actual, err := ConvertBuildSteps(context.Background(), annotationStep.Substep, false, "", "")
			So(err, ShouldBeNil)
			expected := expectedStepsFn(
				func(logName string) string { return logName },
				func(logName string) string { return "" },
			)
			So(actual, ShouldResembleProto, expected)
		})
	})
}

func TestConvertRootStep(t *testing.T) {
	t.Parallel()

	Convey("convert", t, func() {
		rootStep := &annotpb.Step{
			Started: &timestamppb.Timestamp{Seconds: 1400000000},
			Ended:   &timestamppb.Timestamp{Seconds: 1500000000},
			Status:  annotpb.Status_SUCCESS,
			Substep: asSubSteps(
				&annotpb.Step{
					Name:    "cool step",
					Status:  annotpb.Status_SUCCESS,
					Started: &timestamppb.Timestamp{Seconds: 1400000000},
					Ended:   &timestamppb.Timestamp{Seconds: 1500000000},
					Property: []*annotpb.Step_Property{
						{
							Name:  "string_prop",
							Value: "\"baz\"",
						},
					},
				},
			),
			StdoutStream: &annotpb.LogdogStream{Name: "build/stdout"},
			OtherLinks: []*annotpb.AnnotationLink{
				{
					Label: "awesome_log",
					Value: &annotpb.AnnotationLink_LogdogStream{
						LogdogStream: &annotpb.LogdogStream{Name: "build/awesome"},
					},
				},
			},
			Text: []string{
				"text one",
				"text two",
			},
			Property: []*annotpb.Step_Property{
				{
					Name:  "map_prop",
					Value: "{\"foo\" : \"bar\"}",
				},
			},
		}

		expectedBuild := &pb.Build{
			StartTime: &timestamppb.Timestamp{Seconds: 1400000000},
			EndTime:   &timestamppb.Timestamp{Seconds: 1500000000},
			Status:    pb.Status_SUCCESS,
			Steps: []*pb.Step{
				{
					Name:      "cool step",
					Status:    pb.Status_SUCCESS,
					StartTime: &timestamppb.Timestamp{Seconds: 1400000000},
					EndTime:   &timestamppb.Timestamp{Seconds: 1500000000},
				},
			},
			SummaryMarkdown: "\n\n<div>text one text two</div>\n\n",
			Output: &pb.Build_Output{
				Logs: []*pb.Log{
					{
						Name: "stdout",
						Url:  "build/stdout",
					},
					{
						Name: "awesome_log",
						Url:  "build/awesome",
					},
				},
				Properties: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"map_prop": {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"foo": {
											Kind: &structpb.Value_StringValue{
												StringValue: "bar",
											},
										},
									},
								},
							},
						},
						"string_prop": {
							Kind: &structpb.Value_StringValue{
								StringValue: "baz",
							},
						},
					},
				},
				SummaryMarkdown: "\n\n<div>text one text two</div>\n\n",
				Status:          pb.Status_SUCCESS,
			},
		}

		var test = func() {
			actual, err := ConvertRootStep(context.Background(), rootStep)
			So(err, ShouldBeNil)
			So(actual, ShouldResembleProto, expectedBuild)
		}

		test()

		Convey("infra failure build", func() {
			rootStep.Status = annotpb.Status_FAILURE
			rootStep.FailureDetails = &annotpb.FailureDetails{
				Type: annotpb.FailureDetails_INFRA,
				Text: "bad infra failure",
			}

			expectedBuild.Status = pb.Status_INFRA_FAILURE
			expectedBuild.SummaryMarkdown = "bad infra failure\n\n" + expectedBuild.SummaryMarkdown
			expectedBuild.Output.Status = pb.Status_INFRA_FAILURE
			expectedBuild.Output.SummaryMarkdown = expectedBuild.SummaryMarkdown
			test()
		})
		Convey("worst step status", func() {
			rootStep.Substep[0].GetStep().Status = annotpb.Status_FAILURE

			expectedBuild.Steps[0].Status = pb.Status_FAILURE
			expectedBuild.Status = pb.Status_FAILURE
			expectedBuild.Output.Status = pb.Status_FAILURE
			test()
		})
		Convey("use largest step end time", func() {
			rootStep.Substep[0].GetStep().Ended = &timestamppb.Timestamp{Seconds: 1600000000}

			expectedBuild.Steps[0].EndTime = &timestamppb.Timestamp{Seconds: 1600000000}
			expectedBuild.EndTime = &timestamppb.Timestamp{Seconds: 1600000000}
			test()
		})

	})
}

func asSubSteps(subSteps ...*annotpb.Step) []*annotpb.Step_Substep {
	ret := make([]*annotpb.Step_Substep, len(subSteps))
	for i, subStep := range subSteps {
		ret[i] = &annotpb.Step_Substep{
			Substep: &annotpb.Step_Substep_Step{
				Step: subStep,
			},
		}
	}
	return ret
}
