// Copyright 2026 The LUCI Authors.
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

package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	lucinotifypb "go.chromium.org/luci/luci_notify/api/service/v1"
)

type mockBuildersClient struct {
	lucinotifypb.BuildersClient
	list func(ctx context.Context, in *lucinotifypb.ListBuildersRequest) (*lucinotifypb.ListBuildersResponse, error)
}

func (m *mockBuildersClient) List(ctx context.Context, in *lucinotifypb.ListBuildersRequest, opts ...grpc.CallOption) (*lucinotifypb.ListBuildersResponse, error) {
	return m.list(ctx, in)
}

func TestBuildBuilderListFilter(t *testing.T) {
	t.Parallel()

	ftt.Run("build_builder_list_filter", t, func(t *ftt.Test) {
		tests := []struct {
			name      string
			rotation  string
			builder   string
			statuses  []string
			rawFilter string
			want      string
		}{
			{
				name:     "rotation_underscore_hyphen_or_dot",
				rotation: "chrome_browser_release",
				statuses: []string{"failure", "infra_failure"},
				want:     `(on_call_rotations = "chrome_browser_release" OR on_call_rotations = "chrome-browser-release" OR on_call_rotations = "chrome.browser.release") AND (status = "FAILURE" OR status = "INFRA_FAILURE")`,
			},
			{
				name:     "rotation_with_uppercase_resource_prefix_and_dotted_variant",
				rotation: "Rotations/chromium-gpu",
				statuses: []string{"infra-failure", "failed"},
				want:     `(on_call_rotations = "chromium-gpu" OR on_call_rotations = "chromium_gpu" OR on_call_rotations = "chromium.gpu") AND (status = "INFRA_FAILURE" OR status = "FAILURE")`,
			},
			{
				name:     "mixed_separator_rotation_keeps_exact_first",
				rotation: "chromium.gpu_extra",
				statuses: []string{"passed,cancelled,infra,fail,succeeded,pass,cancel"},
				want:     `(on_call_rotations = "chromium.gpu_extra" OR on_call_rotations = "chromium_gpu_extra" OR on_call_rotations = "chromium-gpu-extra" OR on_call_rotations = "chromium.gpu.extra") AND (status = "SUCCESS" OR status = "CANCELED" OR status = "INFRA_FAILURE" OR status = "FAILURE")`,
			},
			{
				name:      "three_part_builder_and_raw_filter",
				builder:   "chromium-m156/ci/Linux Tests",
				statuses:  []string{"FAILURE"},
				rawFilter: `on_call_rotations = "chromium"`,
				want:      `project = "chromium-m156" AND bucket = "ci" AND builder = "Linux Tests" AND status = "FAILURE" AND (on_call_rotations = "chromium")`,
			},
			{
				name:    "project_and_bucket_builder_filter",
				builder: "chromium-m156/ci",
				want:    `project = "chromium-m156" AND bucket = "ci"`,
			},
			{
				name:    "project_only_builder_filter",
				builder: "chromium",
				want:    `project = "chromium"`,
			},
			{
				name:     "comma_separated_statuses_deduplicated",
				rotation: "chromium",
				statuses: []string{"FAILURE,INFRA_FAILURE,failure"},
				want:     `on_call_rotations = "chromium" AND (status = "FAILURE" OR status = "INFRA_FAILURE")`,
			},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *ftt.Test) {
				f, err := buildBuilderListFilter(tc.rotation, tc.builder, tc.statuses, tc.rawFilter)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, f, should.Equal(tc.want))
			})
		}

		t.Run("errors_on_empty_or_invalid_inputs", func(t *ftt.Test) {
			_, err := buildBuilderListFilter("", "", nil, "")
			assert.Loosely(t, err, should.NotBeNil)

			_, err = buildBuilderListFilter("Rotations/", "", nil, "")
			assert.Loosely(t, err, should.NotBeNil)

			for _, badStatus := range []string{"NOT_A_STATUS", "STARTED", "SCHEDULED", "ENDED_MASK", "", ","} {
				_, err = buildBuilderListFilter("chromium", "", []string{badStatus}, "")
				assert.Loosely(t, err, should.NotBeNil)
			}

			for _, badBuilder := range []string{"/", "chromium//Linux Tests", "a/b/c/d"} {
				_, err = buildBuilderListFilter("", badBuilder, nil, "")
				assert.Loosely(t, err, should.NotBeNil)
			}
		})
	})
}

func TestBuilderListCommand(t *testing.T) {
	t.Parallel()

	ts := timestamppb.New(time.Date(2026, 11, 5, 4, 30, 0, 0, time.UTC))

	ftt.Run("luci_notify_builder_list", t, func(t *ftt.Test) {
		var capturedFilter string
		var calls int
		mockClient := &mockBuildersClient{
			list: func(ctx context.Context, in *lucinotifypb.ListBuildersRequest) (*lucinotifypb.ListBuildersResponse, error) {
				calls++
				capturedFilter = in.Filter
				if in.PageToken == "" {
					return &lucinotifypb.ListBuildersResponse{
						Builders: []*lucinotifypb.BuilderStatus{
							{
								Name:            "chromium-m156/ci/Linux Tests",
								Project:         "chromium-m156",
								Bucket:          "ci",
								Builder:         "Linux Tests",
								Status:          "FAILURE",
								BuildId:         8682736451234567890,
								UpdateTime:      ts,
								OnCallRotations: []string{"chrome_browser_release"},
							},
						},
						NextPageToken: "page2",
					}, nil
				}
				return &lucinotifypb.ListBuildersResponse{
					Builders: []*lucinotifypb.BuilderStatus{
						{
							Name:            "chromium-m156/ci/Mac Tests",
							Project:         "chromium-m156",
							Bucket:          "ci",
							Builder:         "Mac Tests",
							Status:          "INFRA_FAILURE",
							BuildId:         8682736451234567891,
							UpdateTime:      ts,
							OnCallRotations: []string{"chrome_browser_release"},
						},
					},
				}, nil
			},
		}

		t.Run("human_readable_output_with_positional_url_and_pagination", func(t *ftt.Test) {
			cmd := BuilderListCmd(base.NewAuthFlags())
			run := cmd.CommandRun().(*builderListRun)
			var out bytes.Buffer
			run.out = &out
			run.buildersClient = mockClient

			code := run.Run(&cli.Application{Name: "luci"}, []string{
				"-status", "FAILURE,INFRA_FAILURE",
				"https://ci.chromium.org/ui/labs/monitoring/chrome-browser-release",
			}, nil)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, calls, should.Equal(2))
			assert.Loosely(t, capturedFilter, should.Equal(`(on_call_rotations = "chrome-browser-release" OR on_call_rotations = "chrome_browser_release" OR on_call_rotations = "chrome.browser.release") AND (status = "FAILURE" OR status = "INFRA_FAILURE")`))

			got := out.String()
			assert.Loosely(t, got, should.ContainSubstring("Rotation:          chrome-browser-release"))
			assert.Loosely(t, got, should.ContainSubstring("Builders (2):"))
			assert.Loosely(t, got, should.ContainSubstring("Builder:       chromium-m156/ci/Linux Tests"))
			assert.Loosely(t, got, should.ContainSubstring("Flags:         -buildid 8682736451234567890 -invocationid build-8682736451234567890"))
			assert.Loosely(t, got, should.ContainSubstring("luci build get -buildid 8682736451234567890"))
			assert.Loosely(t, got, should.ContainSubstring("luci verdict list -invocationid build-8682736451234567890"))
		})

		t.Run("json_output_matches_protojson_schema_and_respects_limit", func(t *ftt.Test) {
			calls = 0
			cmd := BuilderListCmd(base.NewAuthFlags())
			run := cmd.CommandRun().(*builderListRun)
			assert.Loosely(t, run.limit, should.Equal(20))
			var out, errOut bytes.Buffer
			run.out = &out
			run.errOut = &errOut
			run.buildersClient = mockClient

			code := run.Run(&cli.Application{Name: "luci"}, []string{
				"-rotation", "chrome_browser_release",
				"-limit", "1",
				"-json",
			}, nil)
			assert.Loosely(t, code, should.BeZero)
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, errOut.String(), should.ContainSubstring("(showing 1 builders; use -limit 0 to fetch all)"))

			var parsed map[string]any
			assert.Loosely(t, json.Unmarshal(out.Bytes(), &parsed), should.BeNil)
			buildersRaw, ok := parsed["builders"].([]any)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, len(buildersRaw), should.Equal(1))
			first := buildersRaw[0].(map[string]any)
			assert.Loosely(t, first["name"], should.Equal("chromium-m156/ci/Linux Tests"))
			assert.Loosely(t, first["status"], should.Equal("FAILURE"))

			calls = 0
			cmdUnfiltered := BuilderListCmd(base.NewAuthFlags())
			runUnfiltered := cmdUnfiltered.CommandRun().(*builderListRun)
			var unfilteredOut bytes.Buffer
			runUnfiltered.out = &unfilteredOut
			runUnfiltered.buildersClient = mockClient
			assert.Loosely(t, runUnfiltered.Run(&cli.Application{Name: "luci"}, []string{
				"-rotation", "chrome_browser_release",
				"-limit", "1",
			}, nil), should.BeZero)
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, unfilteredOut.String(), should.ContainSubstring("Builders (showing 1; use -status FAILURE,INFRA_FAILURE to filter failures or -limit 0 to see all):"))

			calls = 0
			cmdFiltered := BuilderListCmd(base.NewAuthFlags())
			runFiltered := cmdFiltered.CommandRun().(*builderListRun)
			var filteredOut bytes.Buffer
			runFiltered.out = &filteredOut
			runFiltered.buildersClient = mockClient
			assert.Loosely(t, runFiltered.Run(&cli.Application{Name: "luci"}, []string{
				"-rotation", "chrome_browser_release",
				"-status", "FAILURE,INFRA_FAILURE",
				"-limit", "1",
			}, nil), should.BeZero)
			assert.Loosely(t, calls, should.Equal(1))
			assert.Loosely(t, filteredOut.String(), should.ContainSubstring("Builders (showing 1; use -limit 0 to see all):"))
		})

		t.Run("positional_rotation_and_builder_flag_targets", func(t *ftt.Test) {
			targetTests := []struct {
				name       string
				args       []string
				wantFilter string
			}{
				{
					name:       "canonical_rotation_resource_name",
					args:       []string{"rotations/chrome-browser-release"},
					wantFilter: `(on_call_rotations = "chrome-browser-release" OR on_call_rotations = "chrome_browser_release" OR on_call_rotations = "chrome.browser.release")`,
				},
				{
					name:       "monitoring_alerts_url_extracts_rotation",
					args:       []string{"https://ci.chromium.org/ui/labs/monitoring/chrome-browser-release/alerts/chrome%2Fm146%2Fmac64%2Fcompile"},
					wantFilter: `(on_call_rotations = "chrome-browser-release" OR on_call_rotations = "chrome_browser_release" OR on_call_rotations = "chrome.browser.release")`,
				},
				{
					name:       "three_segment_builder_flag",
					args:       []string{"-builder", "chrome/m146/mac64"},
					wantFilter: `project = "chrome" AND bucket = "m146" AND builder = "mac64"`,
				},
			}
			for _, tc := range targetTests {
				t.Run(tc.name, func(t *ftt.Test) {
					calls = 0
					cmd := BuilderListCmd(base.NewAuthFlags())
					run := cmd.CommandRun().(*builderListRun)
					var out bytes.Buffer
					run.out = &out
					run.buildersClient = mockClient

					code := run.Run(&cli.Application{Name: "luci"}, tc.args, nil)
					assert.Loosely(t, code, should.BeZero)
					assert.Loosely(t, capturedFilter, should.Equal(tc.wantFilter))
				})
			}
		})

		t.Run("rejects_non_rotation_and_alert_group_positional_targets_without_rpc", func(t *ftt.Test) {
			for _, badArgs := range [][]string{
				{"-status", "FAILURE", "https://ci.chromium.org/ui/b/8765432109876543210"},
				{"b/581174243"},
				{"581174243"},
			} {
				calls = 0
				cmd := BuilderListCmd(base.NewAuthFlags())
				run := cmd.CommandRun().(*builderListRun)
				var out, errOut bytes.Buffer
				run.out = &out
				run.errOut = &errOut
				run.buildersClient = mockClient

				code := run.Run(&cli.Application{Name: "luci"}, badArgs, nil)
				assert.Loosely(t, code, should.Equal(1))
				assert.Loosely(t, calls, should.BeZero)
			}

			for _, alertGroupArg := range []string{
				"rotations/chrome-browser-release/alertGroups/m146-compile",
				"https://ci.chromium.org/ui/labs/monitoring/chrome-browser-release/alert-groups/m146-compile",
			} {
				calls = 0
				cmd := BuilderListCmd(base.NewAuthFlags())
				run := cmd.CommandRun().(*builderListRun)
				var out, errOut bytes.Buffer
				run.out = &out
				run.errOut = &errOut
				run.buildersClient = mockClient

				code := run.Run(&cli.Application{Name: "luci"}, []string{alertGroupArg}, nil)
				assert.Loosely(t, code, should.Equal(1))
				assert.Loosely(t, calls, should.BeZero)
				assert.Loosely(t, errOut.String(), should.ContainSubstring("positional target is an alert group, not a rotation"))
				assert.Loosely(t, errOut.String(), should.ContainSubstring("luci notify alert-group get -rotation chrome-browser-release -id m146-compile"))
			}
		})

		t.Run("pagination_stops_on_empty_page", func(t *ftt.Test) {
			emptyLoopCalls := 0
			emptyLoopClient := &mockBuildersClient{
				list: func(ctx context.Context, in *lucinotifypb.ListBuildersRequest) (*lucinotifypb.ListBuildersResponse, error) {
					emptyLoopCalls++
					return &lucinotifypb.ListBuildersResponse{
						NextPageToken: "stuck-token",
					}, nil
				},
			}
			got, hasMore, err := fetchBuilders(t.Context(), emptyLoopClient, `project = "chromium"`, 0)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, hasMore, should.BeFalse)
			assert.Loosely(t, len(got), should.BeZero)
			assert.Loosely(t, emptyLoopCalls, should.Equal(1))
		})
	})
}
