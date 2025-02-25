// Copyright 2019 The LUCI Authors.
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

package protoutil

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestTimestamps(t *testing.T) {
	t.Parallel()

	ftt.Run("Durations", t, func(t *ftt.Test) {
		build := &pb.Build{
			CreateTime: &timestamppb.Timestamp{Seconds: 1000},
			StartTime:  &timestamppb.Timestamp{Seconds: 1010},
			EndTime:    &timestamppb.Timestamp{Seconds: 1030},
		}
		dur, ok := SchedulingDuration(build)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, dur, should.Equal(10*time.Second))

		dur, ok = RunDuration(build)
		assert.Loosely(t, ok, should.BeTrue)
		assert.Loosely(t, dur, should.Equal(20*time.Second))
	})
}

func TestSetStatus(t *testing.T) {
	t.Parallel()

	ftt.Run("SetStatus", t, func(t *ftt.Test) {
		build := &pb.Build{}
		now := testclock.TestRecentTimeUTC

		t.Run("STARTED", func(t *ftt.Test) {
			SetStatus(now, build, pb.Status_STARTED)
			assert.Loosely(t, build, should.Match(&pb.Build{
				Status:     pb.Status_STARTED,
				StartTime:  timestamppb.New(now),
				UpdateTime: timestamppb.New(now),
			}))

			t.Run("no-op", func(t *ftt.Test) {
				SetStatus(now.Add(time.Minute), build, pb.Status_STARTED)
				assert.Loosely(t, build, should.Match(&pb.Build{
					Status:     pb.Status_STARTED,
					StartTime:  timestamppb.New(now),
					UpdateTime: timestamppb.New(now),
				}))
			})
		})

		t.Run("CANCELED", func(t *ftt.Test) {
			SetStatus(now, build, pb.Status_CANCELED)
			assert.Loosely(t, build, should.Match(&pb.Build{
				Status:     pb.Status_CANCELED,
				UpdateTime: timestamppb.New(now),
				EndTime:    timestamppb.New(now),
			}))
		})
	})
}

func TestExePayloadPath(t *testing.T) {
	t.Parallel()

	ftt.Run("from agent.purposes", t, func(t *ftt.Test) {
		b := &pb.Build{
			Infra: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
							"kitchen-checkout": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
						},
					},
				},
			},
		}
		assert.Loosely(t, ExePayloadPath(b), should.Equal("kitchen-checkout"))
	})

	ftt.Run("from agent.purposes", t, func(t *ftt.Test) {
		b := &pb.Build{
			Infra: &pb.BuildInfra{
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir:    "cache",
					PayloadPath: "kitchen-checkout",
				},
			},
		}
		assert.Loosely(t, ExePayloadPath(b), should.Equal("kitchen-checkout"))
	})
}

func TestMergeSummary(t *testing.T) {
	t.Parallel()

	ftt.Run("Variants of MergeSummary", t, func(t *ftt.Test) {
		t.Run("No cancel message", func(t *ftt.Test) {
			b := &pb.Build{
				SummaryMarkdown: "summary",
			}
			assert.Loosely(t, MergeSummary(b), should.Equal("summary"))
		})

		t.Run("No summary message", func(t *ftt.Test) {
			b := &pb.Build{
				CancellationMarkdown: "cancellation",
			}
			assert.Loosely(t, MergeSummary(b), should.Equal("cancellation"))
		})

		t.Run("Summary and cancel message", func(t *ftt.Test) {
			b := &pb.Build{
				SummaryMarkdown:      "summary",
				CancellationMarkdown: "cancellation",
			}
			assert.Loosely(t, MergeSummary(b), should.Equal("summary\ncancellation"))
		})

		t.Run("Summary and task message", func(t *ftt.Test) {
			b := &pb.Build{
				SummaryMarkdown: "summary",
				Infra: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							SummaryMarkdown: "bot_died",
						},
					},
				},
			}
			assert.Loosely(t, MergeSummary(b), should.Equal("summary\nbot_died"))
		})

		t.Run("Neither summary nor CancelMessage", func(t *ftt.Test) {
			b := &pb.Build{}
			assert.Loosely(t, MergeSummary(b), should.BeEmpty)
		})

		t.Run("merged summary too long", func(t *ftt.Test) {
			b := &pb.Build{
				SummaryMarkdown: strings.Repeat("l", SummaryMarkdownMaxLength+1),
			}
			assert.Loosely(t, MergeSummary(b), should.Equal(strings.Repeat("l", SummaryMarkdownMaxLength-3)+"..."))
		})

		t.Run("Summary duplication", func(t *ftt.Test) {
			b := &pb.Build{
				SummaryMarkdown: "summary",
				Output: &pb.Build_Output{
					SummaryMarkdown: "summary",
				},
				CancellationMarkdown: "cancellation",
			}
			assert.Loosely(t, MergeSummary(b), should.Equal("summary\ncancellation"))
		})
	})
}

func TestBotDimensions(t *testing.T) {
	t.Parallel()

	botDims := []*pb.StringPair{
		{
			Key:   "cpu",
			Value: "x86",
		},
		{
			Key:   "cpu",
			Value: "x86-64",
		},
		{
			Key:   "os",
			Value: "Linux",
		},
	}

	b := &pb.Build{
		Infra: &pb.BuildInfra{},
	}
	ftt.Run("build on swarming", t, func(t *ftt.Test) {
		b.Infra.Swarming = &pb.BuildInfra_Swarming{
			BotDimensions: botDims,
		}
		assert.Loosely(t, MustBotDimensions(b), should.Match(botDims))
	})

	ftt.Run("build on backend", t, func(t *ftt.Test) {
		b.Infra.Backend = &pb.BuildInfra_Backend{
			Task: &pb.Task{
				Details: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bot_dimensions": {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"cpu": {
											Kind: &structpb.Value_ListValue{
												ListValue: &structpb.ListValue{
													Values: []*structpb.Value{
														{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
														{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
													},
												},
											},
										},
										"os": {
											Kind: &structpb.Value_ListValue{
												ListValue: &structpb.ListValue{
													Values: []*structpb.Value{
														{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		assert.Loosely(t, MustBotDimensions(b), should.Match(botDims))
	})
}

func TestAddBotDimensionsToTaskDetails(t *testing.T) {
	t.Parallel()
	ftt.Run("with empty details", t, func(t *ftt.Test) {
		botDims := []*pb.StringPair{
			{
				Key:   "cpu",
				Value: "x86",
			},
			{
				Key:   "cpu",
				Value: "x86-64",
			},
			{
				Key:   "os",
				Value: "Linux",
			},
		}
		expected := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"bot_dimensions": {
					Kind: &structpb.Value_StructValue{
						StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"cpu": {
									Kind: &structpb.Value_ListValue{
										ListValue: &structpb.ListValue{
											Values: []*structpb.Value{
												{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
												{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
											},
										},
									},
								},
								"os": {
									Kind: &structpb.Value_ListValue{
										ListValue: &structpb.ListValue{
											Values: []*structpb.Value{
												{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		actual, err := AddBotDimensionsToTaskDetails(botDims, nil)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(expected))
	})
	ftt.Run("with existing details", t, func(t *ftt.Test) {
		botDims := []*pb.StringPair{
			{
				Key:   "cpu",
				Value: "x86",
			},
			{
				Key:   "cpu",
				Value: "x86-64",
			},
			{
				Key:   "os",
				Value: "Linux",
			},
		}
		details := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"a": {
					Kind: &structpb.Value_StringValue{StringValue: "x86"},
				},
			},
		}
		expected := &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"a": {
					Kind: &structpb.Value_StringValue{StringValue: "x86"},
				},
				"bot_dimensions": {
					Kind: &structpb.Value_StructValue{
						StructValue: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"cpu": {
									Kind: &structpb.Value_ListValue{
										ListValue: &structpb.ListValue{
											Values: []*structpb.Value{
												{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
												{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
											},
										},
									},
								},
								"os": {
									Kind: &structpb.Value_ListValue{
										ListValue: &structpb.ListValue{
											Values: []*structpb.Value{
												{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}
		actual, err := AddBotDimensionsToTaskDetails(botDims, details)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(expected))
	})
}
