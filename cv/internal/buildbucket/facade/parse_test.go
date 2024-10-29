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

package bbfacade

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/cv/api/recipe/v1"
	bbfake "go.chromium.org/luci/cv/internal/buildbucket/fake"
	"go.chromium.org/luci/cv/internal/tryjob"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseStatusAndResult(t *testing.T) {
	ftt.Run("Parse Status and Result", t, func(t *ftt.Test) {
		const buildID = 12345
		const buildSummary = "foo"
		builder := &bbpb.BuilderID{
			Project: "aProject",
			Bucket:  "aBucket",
			Builder: "aBuilder",
		}
		createTime := testclock.TestRecentTimeUTC

		b := bbfake.NewBuildConstructor().
			WithHost("example.buildbucket.com").
			WithID(buildID).
			WithBuilderID(builder).
			WithCreateTime(createTime).
			WithStatus(bbpb.Status_SCHEDULED).
			WithSummaryMarkdown(buildSummary).
			WithInvocation("resultdb.example.com", "invocation/build:12345").
			Construct()
		ctx := context.Background()

		t.Run("Returns an error", func(t *ftt.Test) {
			t.Run("On an invalid build status", func(t *ftt.Test) {
				b.Status = bbpb.Status_ENDED_MASK
				_, _, err := parseStatusAndResult(ctx, b)
				assert.Loosely(t, err, should.ErrLike("unexpected buildbucket status"))
			})
		})
		t.Run("Parses a valid build proto", func(t *ftt.Test) {
			t.Run("For an ended build", func(t *ftt.Test) {
				startTime := createTime.Add(1 * time.Minute)
				endTime := createTime.Add(2 * time.Minute)
				b = bbfake.NewConstructorFromBuild(b).
					WithStartTime(startTime).
					WithEndTime(endTime).
					WithUpdateTime(endTime).
					Construct()
				t.Run("That succeeded", func(t *ftt.Test) {
					b = bbfake.NewConstructorFromBuild(b).WithStatus(bbpb.Status_SUCCESS).Construct()
					status, result, err := parseStatusAndResult(ctx, b)
					assert.NoErr(t, err)
					assert.Loosely(t, status, should.Equal(tryjob.Status_ENDED))
					assert.Loosely(t, result.Status, should.Equal(tryjob.Result_SUCCEEDED))
					assert.Loosely(t, result, should.Resemble(&tryjob.Result{
						Status:     tryjob.Result_SUCCEEDED,
						CreateTime: timestamppb.New(createTime),
						UpdateTime: timestamppb.New(endTime),
						Backend: &tryjob.Result_Buildbucket_{
							Buildbucket: &tryjob.Result_Buildbucket{
								Id:              buildID,
								Builder:         builder,
								Status:          bbpb.Status_SUCCESS,
								SummaryMarkdown: buildSummary,
								Infra: &bbpb.BuildInfra{
									Resultdb: &bbpb.BuildInfra_ResultDB{
										Hostname:   "resultdb.example.com",
										Invocation: "invocation/build:12345",
									},
								},
							},
						},
					}))
				})
				t.Run("That timed out", func(t *ftt.Test) {
					b = bbfake.NewConstructorFromBuild(b).
						WithStatus(bbpb.Status_FAILURE).
						WithTimeout(true).
						Construct()
					status, result, err := parseStatusAndResult(ctx, b)
					assert.NoErr(t, err)
					assert.Loosely(t, status, should.Equal(tryjob.Status_ENDED))
					assert.Loosely(t, result.GetStatus(), should.Equal(tryjob.Result_TIMEOUT))
				})
				t.Run("That failed", func(t *ftt.Test) {
					t.Run("Transiently", func(t *ftt.Test) {
						b = bbfake.NewConstructorFromBuild(b).WithStatus(bbpb.Status_INFRA_FAILURE).Construct()
						status, result, err := parseStatusAndResult(ctx, b)
						assert.NoErr(t, err)
						assert.Loosely(t, status, should.Equal(tryjob.Status_ENDED))
						assert.Loosely(t, result.GetStatus(), should.Equal(tryjob.Result_FAILED_TRANSIENTLY))
					})
					t.Run("Permanently", func(t *ftt.Test) {
						b.Status = bbpb.Status_FAILURE
						status, result, err := parseStatusAndResult(ctx, b)
						assert.Loosely(t, status, should.Equal(tryjob.Status_ENDED))
						assert.NoErr(t, err)
						assert.Loosely(t, result.GetStatus(), should.Equal(tryjob.Result_FAILED_PERMANENTLY))
					})
				})
				t.Run("That cancelled", func(t *ftt.Test) {
					b = bbfake.NewConstructorFromBuild(b).WithStatus(bbpb.Status_INFRA_FAILURE).Construct()
					status, result, err := parseStatusAndResult(ctx, b)
					assert.NoErr(t, err)
					assert.Loosely(t, status, should.Equal(tryjob.Status_ENDED))
					assert.Loosely(t, result.GetStatus(), should.Equal(tryjob.Result_FAILED_TRANSIENTLY))
				})
			})
			t.Run("For a pending build", func(t *ftt.Test) {
				t.Run("That is still scheduled", func(t *ftt.Test) {
					status, result, err := parseStatusAndResult(ctx, b)
					assert.NoErr(t, err)
					assert.Loosely(t, status, should.Equal(tryjob.Status_TRIGGERED))
					assert.Loosely(t, result.Status, should.Equal(tryjob.Result_UNKNOWN))
				})
				t.Run("That is already running", func(t *ftt.Test) {
					b = bbfake.NewConstructorFromBuild(b).
						WithStatus(bbpb.Status_STARTED).
						WithStartTime(createTime.Add(1 * time.Minute)).
						Construct()
					status, result, err := parseStatusAndResult(ctx, b)
					assert.NoErr(t, err)
					assert.Loosely(t, status, should.Equal(tryjob.Status_TRIGGERED))
					assert.Loosely(t, result.Status, should.Equal(tryjob.Result_UNKNOWN))
				})
			})
		})
	})
}

func TestParseOutput(t *testing.T) {
	ctx := context.Background()
	ftt.Run("parseOutput", t, func(t *ftt.Test) {
		t.Run("Allow reuse", func(t *ftt.Test) {
			t.Run("For full runs", func(t *ftt.Test) {
				result := parseBuildResult(ctx, loadTestBuild("reuse_full"))
				assert.Loosely(t, result.output, should.Resemble(&recipe.Output{
					Reuse: []*recipe.Output_Reuse{{ModeRegexp: "FULL_RUN"}},
				}))
				assert.Loosely(t, result.isTransFailure, should.BeFalse)
				assert.That(t, result.err, should.Equal[*validation.Error](nil))
			})
			t.Run("For dry runs", func(t *ftt.Test) {
				result := parseBuildResult(ctx, loadTestBuild("reuse_dry"))
				assert.Loosely(t, result.output, should.Resemble(&recipe.Output{
					Reuse: []*recipe.Output_Reuse{{ModeRegexp: "DRY_RUN"}},
				}))
				assert.Loosely(t, result.isTransFailure, should.BeFalse)
				assert.That(t, result.err, should.Equal[*validation.Error](nil))
			})
		})
		t.Run("Do not retry", func(t *ftt.Test) {
			t.Run("Legacy property only", func(t *ftt.Test) {
				result := parseBuildResult(ctx, loadTestBuild("retry_denied_legacy"))
				assert.Loosely(t, result.output, should.Resemble(&recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}))
				assert.Loosely(t, result.isTransFailure, should.BeFalse)
				assert.That(t, result.err, should.Equal[*validation.Error](nil))
			})
			t.Run("Proto property only", func(t *ftt.Test) {
				result := parseBuildResult(ctx, loadTestBuild("retry_denied_new"))
				assert.Loosely(t, result.output, should.Resemble(&recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_DENIED}))
				assert.Loosely(t, result.isTransFailure, should.BeFalse)
				assert.That(t, result.err, should.Equal[*validation.Error](nil))
			})
			t.Run("Proto overrides legacy", func(t *ftt.Test) {
				// In this test, the protobuf-based property allows retry and
				// the legacy property denies it.
				// Test that the protobuf property overrides the legacy one.
				result := parseBuildResult(ctx, loadTestBuild("retry_denied_conflict"))
				assert.Loosely(t, result.output, should.Resemble(&recipe.Output{Retry: recipe.Output_OUTPUT_RETRY_ALLOWED}))
				assert.Loosely(t, result.isTransFailure, should.BeFalse)
				assert.That(t, result.err, should.Equal[*validation.Error](nil))
			})
		})
		t.Run("Transient failure", func(t *ftt.Test) {
			result := parseBuildResult(ctx, loadTestBuild("transient_failure"))
			assert.Loosely(t, result.output, should.Resemble(&recipe.Output{}))
			assert.Loosely(t, result.isTransFailure, should.BeTrue)
			assert.That(t, result.err, should.Equal[*validation.Error](nil))
		})
		t.Run("No properties", func(t *ftt.Test) {
			result := parseBuildResult(ctx, loadTestBuild("no_props"))
			assert.Loosely(t, result.output, should.BeNil)
			assert.Loosely(t, result.isTransFailure, should.BeFalse)
			assert.That(t, result.err, should.Equal[*validation.Error](nil))
		})
		t.Run("Bad data", func(t *ftt.Test) {
			result := parseBuildResult(ctx, loadTestBuild("bad_data"))
			assert.Loosely(t, result.output, should.Resemble(&recipe.Output{}))
			assert.Loosely(t, result.isTransFailure, should.BeFalse)
			assert.Loosely(t, result.err.Errors, should.HaveLength(2))
		})
	})
}

func loadTestBuild(fixtureBaseName string) *bbpb.Build {
	data, err := os.ReadFile(fmt.Sprintf("testdata/%s.json", fixtureBaseName))
	if err != nil {
		panic(err)
	}
	ret := &bbpb.Build{}
	if err := protojson.Unmarshal(data, ret); err != nil {
		panic(err)
	}
	return ret
}
