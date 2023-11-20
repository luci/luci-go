// Copyright 2022 The LUCI Authors.
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

package testvariantbqexporter

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testutil"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	RegisterTaskClass()
}

func TestSchedule(t *testing.T) {
	Convey(`TestSchedule`, t, func() {
		ctx, skdr := tq.TestingContext(testutil.TestingContext(), nil)

		realm := "realm"
		cloudProject := "cloudProject"
		dataset := "dataset"
		table := "table"
		predicate := &atvpb.Predicate{
			Status: atvpb.Status_FLAKY,
		}
		now := clock.Now(ctx)
		timeRange := &pb.TimeRange{
			Earliest: timestamppb.New(now.Add(-time.Hour)),
			Latest:   timestamppb.New(now),
		}
		task := &taskspb.ExportTestVariants{
			Realm:        realm,
			CloudProject: cloudProject,
			Dataset:      dataset,
			Table:        table,
			Predicate:    predicate,
			TimeRange:    timeRange,
		}
		So(Schedule(ctx, realm, cloudProject, dataset, table, predicate, timeRange), ShouldBeNil)
		So(skdr.Tasks().Payloads()[0], ShouldResembleProto, task)
	})
}

func createProjectsConfig() map[string]*configpb.ProjectConfig {
	return map[string]*configpb.ProjectConfig{
		"chromium": {
			Realms: []*configpb.RealmConfig{
				{
					Name: "ci",
					TestVariantAnalysis: &configpb.TestVariantAnalysisConfig{
						BqExports: []*configpb.BigQueryExport{
							{
								Table: &configpb.BigQueryExport_BigQueryTable{
									CloudProject: "test-hrd",
									Dataset:      "chromium",
									Table:        "flaky_test_variants_ci",
								},
							},
							{
								Table: &configpb.BigQueryExport_BigQueryTable{
									CloudProject: "test-hrd",
									Dataset:      "chromium",
									Table:        "flaky_test_variants_ci_copy",
								},
							},
						},
					},
				},
				{
					Name: "try",
					TestVariantAnalysis: &configpb.TestVariantAnalysisConfig{
						BqExports: []*configpb.BigQueryExport{
							{
								Table: &configpb.BigQueryExport_BigQueryTable{
									CloudProject: "test-hrd",
									Dataset:      "chromium",
									Table:        "flaky_test_variants_try",
								},
							},
						},
					},
				},
			},
		},
		"project_no_realms": {},
		"project_no_bq": {
			Realms: []*configpb.RealmConfig{
				{
					Name: "ci",
				},
			},
		},
	}
}

func TestScheduleTasks(t *testing.T) {
	Convey(`TestScheduleTasks`, t, func() {
		ctx, skdr := tq.TestingContext(testutil.TestingContext(), nil)
		ctx = memory.Use(ctx)
		config.SetTestProjectConfig(ctx, createProjectsConfig())

		err := ScheduleTasks(ctx)
		So(err, ShouldBeNil)
		So(len(skdr.Tasks().Payloads()), ShouldEqual, 3)
	})
}
