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

package admin

import (
	"context"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	adminpb "go.chromium.org/luci/analysis/internal/admin/proto"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/services/testvariantbqexporter"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	testvariantbqexporter.RegisterTaskClass()
}

func createProjectsConfig() map[string]*configpb.ProjectConfig {
	return map[string]*configpb.ProjectConfig{
		"chromium": {
			Realms: []*configpb.RealmConfig{
				{
					Name: "try",
					TestVariantAnalysis: &configpb.TestVariantAnalysisConfig{
						BqExports: []*configpb.BigQueryExport{
							{
								Table: &configpb.BigQueryExport_BigQueryTable{
									CloudProject: "cloudProject",
									Dataset:      "dataset",
									Table:        "table",
								},
								Predicate: &atvpb.Predicate{
									Status: atvpb.Status_FLAKY,
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestCheckAllowed(t *testing.T) {
	t.Parallel()

	Convey("with access", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:admin@example.com",
			IdentityGroups: []string{allowGroup},
		})
		So(checkAllowed(ctx, ""), ShouldBeNil)
	})

	Convey("no login", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity: "anonymous:anonymous",
		})
		So(checkAllowed(ctx, ""), ShouldErrLike, "not a member of service-luci-analysis-admins")
	})

	Convey("without access", t, func() {
		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			Identity:       "user:user@example.com",
			IdentityGroups: []string{"unrelated_group"},
		})
		So(checkAllowed(ctx, ""), ShouldErrLike, "not a member of service-luci-analysis-admins")
	})
}

func TestValidateExportTestVariantsRequest(t *testing.T) {
	t.Parallel()
	Convey("TestValidateExportTestVariantsRequest", t, func() {
		ctx := context.Background()
		ctx = memory.Use(ctx)
		So(config.SetTestProjectConfig(ctx, createProjectsConfig()), ShouldBeNil)
		const realm = "chromium:try"
		const cloudProject = "cloudProject"
		const dataset = "dataset"
		const table = "table"
		start := time.Date(2021, 11, 12, 0, 0, 0, 0, time.UTC)
		end := start.Add(24 * time.Hour)

		Convey("pass", func() {
			err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
				Realm:        realm,
				CloudProject: cloudProject,
				Dataset:      dataset,
				Table:        table,
				TimeRange: &pb.TimeRange{
					Earliest: timestamppb.New(start),
					Latest:   timestamppb.New(end),
				},
			})
			So(err, ShouldBeNil)
		})

		Convey("no realm", func() {
			err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{})
			So(err, ShouldErrLike, "realm is not specified")
		})

		Convey("table", func() {
			Convey("no cloud project", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm: realm,
				})
				So(err, ShouldErrLike, "cloud project is not specified")
			})

			Convey("no dataset", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
				})
				So(err, ShouldErrLike, "dataset is not specified")
			})

			Convey("no table", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
					Dataset:      dataset,
				})
				So(err, ShouldErrLike, "table is not specified")
			})

			Convey("unknown table", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
					Dataset:      dataset,
					Table:        "unknown",
				})
				So(err, ShouldErrLike, "table not found in realm config")
			})
		})

		Convey("time range", func() {
			Convey("no earliest", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
					Dataset:      dataset,
					Table:        table,
					TimeRange:    &pb.TimeRange{},
				})
				So(err, ShouldErrLike, "time_range: earliest: unspecified")
			})

			Convey("no latest", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
					Dataset:      dataset,
					Table:        table,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(start),
					},
				})
				So(err, ShouldErrLike, "time_range: latest: unspecified")
			})

			Convey("earliest is after latest", func() {
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
					Dataset:      dataset,
					Table:        table,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(end),
						Latest:   timestamppb.New(start),
					},
				})
				So(err, ShouldErrLike, "time_range: earliest must be before latest")
			})

			Convey("latest is in the future", func() {
				now := clock.Now(ctx)
				err := validateExportTestVariantsRequest(ctx, &adminpb.ExportTestVariantsRequest{
					Realm:        realm,
					CloudProject: cloudProject,
					Dataset:      dataset,
					Table:        table,
					TimeRange: &pb.TimeRange{
						Earliest: timestamppb.New(start),
						Latest:   timestamppb.New(now.Add(time.Hour)),
					},
				})
				So(err, ShouldErrLike, "time_range: latest must not be in the future")
			})
		})
	})
}

func TestSplitTimeRange(t *testing.T) {
	t.Parallel()
	start := time.Date(2021, 11, 12, 0, 1, 0, 0, time.UTC)
	Convey("earliest and latest too close", t, func() {
		end := start.Add(time.Minute)
		timeRange := &pb.TimeRange{
			Earliest: timestamppb.New(start),
			Latest:   timestamppb.New(end),
		}
		ranges, err := splitTimeRange(timeRange)
		So(err, ShouldBeNil)
		So(len(ranges), ShouldEqual, 0)
	})

	Convey("split", t, func() {
		end := start.Add(2 * time.Hour)
		timeRange := &pb.TimeRange{
			Earliest: timestamppb.New(start),
			Latest:   timestamppb.New(end),
		}
		ranges, err := splitTimeRange(timeRange)
		So(err, ShouldBeNil)
		So(len(ranges), ShouldEqual, 2)

		start0 := start.Truncate(time.Hour)
		exp := []rangeInTime{
			{
				start: start0,
				end:   start0.Add(time.Hour),
			},
			{
				start: start0.Add(time.Hour),
				end:   start0.Add(2 * time.Hour),
			},
		}
		So(ranges, ShouldResemble, exp)
	})
}

func TestExportTestVariants(t *testing.T) {
	ctx, skdr := tq.TestingContext(context.Background(), nil)
	ctx = auth.WithState(ctx, &authtest.FakeState{
		Identity:       "user:admin@example.com",
		IdentityGroups: []string{allowGroup},
	})
	ctx = memory.Use(ctx)
	config.SetTestProjectConfig(ctx, createProjectsConfig())

	const realm = "chromium:try"
	const cloudProject = "cloudProject"
	const dataset = "dataset"
	const table = "table"
	start := time.Date(2021, 11, 12, 0, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)
	req := &adminpb.ExportTestVariantsRequest{
		Realm:        realm,
		CloudProject: cloudProject,
		Dataset:      dataset,
		Table:        table,
		TimeRange: &pb.TimeRange{
			Earliest: timestamppb.New(start),
			Latest:   timestamppb.New(end),
		},
	}
	Convey("ExportTestVariants", t, func() {
		a := CreateServer()
		_, err := a.ExportTestVariants(ctx, req)
		So(err, ShouldBeNil)
		So(len(skdr.Tasks().Payloads()), ShouldEqual, 2)
		tasks := []*taskspb.ExportTestVariants{
			{
				Realm:        realm,
				CloudProject: cloudProject,
				Dataset:      dataset,
				Table:        table,
				Predicate: &atvpb.Predicate{
					Status: atvpb.Status_FLAKY,
				},
				TimeRange: &pb.TimeRange{
					Earliest: timestamppb.New(start),
					Latest:   timestamppb.New(start.Add(time.Hour)),
				},
			},
			{
				Realm:        realm,
				CloudProject: cloudProject,
				Dataset:      dataset,
				Table:        table,
				Predicate: &atvpb.Predicate{
					Status: atvpb.Status_FLAKY,
				},
				TimeRange: &pb.TimeRange{
					Earliest: timestamppb.New(start.Add(time.Hour)),
					Latest:   timestamppb.New(start.Add(2 * time.Hour)),
				},
			},
		}

		payloads := skdr.Tasks().Payloads()
		sort.Slice(payloads, func(i, j int) bool {
			taski := payloads[i].(*taskspb.ExportTestVariants)
			taskj := payloads[j].(*taskspb.ExportTestVariants)
			return taski.TimeRange.Earliest.AsTime().Before(taskj.TimeRange.Earliest.AsTime())
		})

		So(payloads, ShouldResembleProto, tasks)
	})
}
