// Copyright 2023 The LUCI Authors.
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
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/packedintset"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestResultDBInfo(t *testing.T) {
	t.Parallel()

	Convey("ToProto", t, func() {
		Convey("nil", func() {
			r := ResultDBInfo{}
			So(r.ToProto(), ShouldBeNil)
		})
		Convey("ok", func() {
			r := ResultDBInfo{Hostname: "abc.com", Invocation: "1234acb"}
			So(r.ToProto(), ShouldResembleProto, apipb.ResultDBInfo{
				Hostname:   "abc.com",
				Invocation: "1234acb",
			})
		})
	})
}

func TestTaskResult(t *testing.T) {
	t.Parallel()
	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey("With Datastore", t, func() {
		ctx := memory.Use(context.Background())

		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		fullyPopulated := TaskResultSummary{
			TaskResultCommon: TaskResultCommon{
				State:               apipb.TaskState_COMPLETED,
				Modified:            testTime,
				BotVersion:          "bot_version_123",
				BotDimensions:       BotDimensions{"os": []string{"linux"}, "cpu": []string{"x86_64"}},
				BotIdleSince:        datastore.NewUnindexedOptional(testTime.Add(-30 * time.Minute)),
				BotLogsCloudProject: "example-cloud-project",
				ServerVersions:      []string{"v1.0"},
				CurrentTaskSlice:    1,
				Started:             datastore.NewIndexedNullable(testTime.Add(-1 * time.Hour)),
				Completed:           datastore.NewIndexedNullable(testTime),
				DurationSecs:        datastore.NewUnindexedOptional(3600.0),
				ExitCode:            datastore.NewUnindexedOptional(int64(0)),
				Failure:             false,
				InternalFailure:     false,
				StdoutChunks:        10,
				CASOutputRoot: CASReference{
					CASInstance: "cas-instance",
					Digest: CASDigest{
						Hash:      "cas-hash",
						SizeBytes: 1024,
					},
				},
				CIPDPins: CIPDInput{
					Server: "https://example.cipd.server",
					ClientPackage: CIPDPackage{
						PackageName: "client_pkg",
						Version:     "1.0.0",
						Path:        "client",
					},
				},
				ResultDBInfo: ResultDBInfo{
					Hostname:   "results.api.example.dev",
					Invocation: "inv123",
				},
			},
			Key:                  TaskResultSummaryKey(ctx, reqKey),
			BotID:                datastore.NewUnindexedOptional("bot123"),
			Created:              testTime.Add(-2 * time.Hour),
			Tags:                 []string{"tag1", "tag2"},
			RequestName:          "example-request",
			RequestUser:          "user@example.com",
			RequestPriority:      50,
			RequestAuthenticated: "authenticated-user@example.com",
			RequestRealm:         "example-realm",
			RequestPool:          "example-pool",
			RequestBotID:         "bot123",
			PropertiesHash:       datastore.NewIndexedOptional([]byte("prop-hash")),
			TryNumber:            datastore.NewIndexedNullable(int64(1)),
			CostUSD:              0.05,
			CostSavedUSD:         0.00,
			DedupedFrom:          "",
			ExpirationDelay:      datastore.NewUnindexedOptional(0.0),
		}

		// Can round-trip.
		So(datastore.Put(ctx, &fullyPopulated), ShouldBeNil)
		loaded := TaskResultSummary{Key: TaskResultSummaryKey(ctx, reqKey)}
		So(datastore.Get(ctx, &loaded), ShouldBeNil)
		So(loaded, ShouldResemble, fullyPopulated)
	})

	Convey("ToProto", t, func() {
		ctx := memory.Use(context.Background())

		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		Convey("ok", func() {
			trs := TaskResultSummary{
				TaskResultCommon: TaskResultCommon{
					State:               apipb.TaskState_COMPLETED,
					Modified:            testTime,
					BotVersion:          "bot_version_123",
					BotDimensions:       BotDimensions{"os": []string{"linux"}, "cpu": []string{"x86_64"}},
					BotIdleSince:        datastore.NewUnindexedOptional(testTime.Add(-30 * time.Minute)),
					BotLogsCloudProject: "example-cloud-project",
					ServerVersions:      []string{"v1.0"},
					CurrentTaskSlice:    1,
					Started:             datastore.NewIndexedNullable(testTime.Add(-1 * time.Hour)),
					Completed:           datastore.NewIndexedNullable(testTime),
					DurationSecs:        datastore.NewUnindexedOptional(3600.0),
					ExitCode:            datastore.NewUnindexedOptional(int64(0)),
					Failure:             false,
					InternalFailure:     false,
					StdoutChunks:        10,
					CASOutputRoot: CASReference{
						CASInstance: "cas-instance",
						Digest: CASDigest{
							Hash:      "cas-hash",
							SizeBytes: 1024,
						},
					},
					CIPDPins: CIPDInput{
						Server: "https://example.cipd.server",
						ClientPackage: CIPDPackage{
							PackageName: "client_pkg",
							Version:     "1.0.0",
							Path:        "client",
						},
					},
					ResultDBInfo: ResultDBInfo{
						Hostname:   "results.api.example.dev",
						Invocation: "inv123",
					},
					MissingCIPD: []CIPDPackage{
						{
							PackageName: "package",
							Version:     "version",
							Path:        "path",
						},
					},
					MissingCAS: []CASReference{
						{
							CASInstance: "cas-instance2",
							Digest: CASDigest{
								Hash:      "hash",
								SizeBytes: int64(100),
							},
						},
					},
				},
				Key:                  TaskResultSummaryKey(ctx, reqKey),
				BotID:                datastore.NewUnindexedOptional("bot123"),
				Created:              testTime.Add(-2 * time.Hour),
				Tags:                 []string{"tag1", "tag2"},
				RequestName:          "example-request",
				RequestUser:          "user@example.com",
				RequestPriority:      50,
				RequestAuthenticated: "authenticated-user@example.com",
				RequestRealm:         "example-realm",
				RequestPool:          "example-pool",
				RequestBotID:         "bot123",
				PropertiesHash:       datastore.NewIndexedOptional([]byte("prop-hash")),
				TryNumber:            datastore.NewIndexedNullable(int64(1)),
				CostUSD:              0.05,
				CostSavedUSD:         0.00,
				DedupedFrom:          "",
				ExpirationDelay:      datastore.NewUnindexedOptional(0.0),
			}
			So(trs.ToProto(), ShouldResembleProto, &apipb.TaskResultResponse{
				BotDimensions: []*apipb.StringListPair{
					{Key: "cpu", Value: []string{"x86_64"}},
					{Key: "os", Value: []string{"linux"}},
				},
				BotId:               "bot123",
				BotLogsCloudProject: "example-cloud-project",
				BotVersion:          "bot_version_123",
				CasOutputRoot: &apipb.CASReference{
					CasInstance: "cas-instance",
					Digest: &apipb.Digest{
						Hash:      "cas-hash",
						SizeBytes: 1024,
					},
				},
				CipdPins: &apipb.CipdPins{
					ClientPackage: &apipb.CipdPackage{
						PackageName: "client_pkg",
						Version:     "1.0.0",
						Path:        "client",
					},
				},
				CompletedTs:      timestamppb.New(testTime),
				CostsUsd:         []float32{0.05},
				CreatedTs:        timestamppb.New(testTime.Add(-2 * time.Hour)),
				CurrentTaskSlice: int32(1),
				Duration:         float32(3600),
				MissingCas: []*apipb.CASReference{
					{
						CasInstance: "cas-instance2",
						Digest: &apipb.Digest{
							Hash:      "hash",
							SizeBytes: int64(100),
						},
					},
				},
				MissingCipd: []*apipb.CipdPackage{
					{
						PackageName: "package",
						Version:     "version",
						Path:        "path",
					},
				},
				ModifiedTs: timestamppb.New(testTime),
				Name:       "example-request",
				ResultdbInfo: &apipb.ResultDBInfo{
					Hostname:   "results.api.example.dev",
					Invocation: "inv123",
				},
				RunId:          "65aba3a3e6b99311",
				ServerVersions: []string{"v1.0"},
				StartedTs:      timestamppb.New(testTime.Add(-1 * time.Hour)),
				State:          apipb.TaskState_COMPLETED,
				Tags:           []string{"tag1", "tag2"},
				TaskId:         "65aba3a3e6b99310",
				User:           "user@example.com",
			})
		})
		Convey("mostly empty", func() {
			trs := TaskResultSummary{
				TaskResultCommon: TaskResultCommon{
					Modified: testTime,
				},
				Created: testTime.Add(-2 * time.Hour),
				Key:     TaskResultSummaryKey(ctx, reqKey)}
			So(trs.ToProto(), ShouldResembleProto, &apipb.TaskResultResponse{
				CreatedTs:  timestamppb.New(testTime.Add(-2 * time.Hour)),
				ModifiedTs: timestamppb.New(testTime),
				TaskId:     "65aba3a3e6b99310",
			})
		})

	})

	Convey("CostsUSD", t, func() {
		Convey("ok", func() {
			trs := TaskResultSummary{CostUSD: 100.00}
			So(trs.CostsUSD(), ShouldEqual, []float32{float32(100)})
		})

		Convey("nil", func() {
			trs := TaskResultSummary{}
			So(trs.CostsUSD(), ShouldBeNil)
		})
	})

	Convey("PerformanceStats", t, func() {
		ctx := memory.Use(context.Background())
		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		Convey("ok", func() {
			trs := TaskResultSummary{
				Key: TaskResultSummaryKey(ctx, reqKey),
			}
			ps := PerformanceStats{
				Key:                  PerformanceStatsKey(ctx, reqKey),
				BotOverheadSecs:      1,
				CacheTrim:            OperationStats{DurationSecs: 2},
				PackageInstallation:  OperationStats{DurationSecs: 3},
				NamedCachesInstall:   OperationStats{DurationSecs: 4},
				NamedCachesUninstall: OperationStats{DurationSecs: 5},
				Cleanup:              OperationStats{DurationSecs: 6},
				IsolatedDownload:     CASOperationStats{DurationSecs: 7},
				IsolatedUpload:       CASOperationStats{DurationSecs: 7},
			}
			So(datastore.Put(ctx, &trs, &ps), ShouldBeNil)
			resp, err := trs.PerformanceStats(ctx)
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, &apipb.PerformanceStats{
				BotOverhead:          float32(1),
				CacheTrim:            &apipb.OperationStats{Duration: float32(2)},
				PackageInstallation:  &apipb.OperationStats{Duration: float32(3)},
				NamedCachesInstall:   &apipb.OperationStats{Duration: float32(4)},
				NamedCachesUninstall: &apipb.OperationStats{Duration: float32(5)},
				Cleanup:              &apipb.OperationStats{Duration: float32(6)},
				IsolatedDownload:     &apipb.CASOperationStats{Duration: float32(7)},
				IsolatedUpload:       &apipb.CASOperationStats{Duration: float32(7)},
			})
		})

		Convey("nil", func() {
			trs := TaskResultSummary{
				Key: TaskResultSummaryKey(ctx, reqKey),
			}
			resp, err := trs.PerformanceStats(ctx)
			So(err, ShouldErrLike, "datastore: no such entity")
			So(resp, ShouldBeNil)
		})
	})

	Convey("TaskAuthInfo", t, func() {
		ctx := memory.Use(context.Background())
		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		Convey("ok", func() {
			trs := TaskResultSummary{
				Key:                  TaskResultSummaryKey(ctx, reqKey),
				RequestAuthenticated: "authenticated-user@example.com",
				RequestRealm:         "example-realm",
				RequestPool:          "example-pool",
				RequestBotID:         "bot123",
			}

			So(trs.TaskAuthInfo(), ShouldEqual, acls.TaskAuthInfo{
				TaskID:    "65aba3a3e6b99310",
				Realm:     "example-realm",
				Pool:      "example-pool",
				BotID:     "bot123",
				Submitter: identity.Identity("authenticated-user@example.com"),
			})
		})

		Convey("mostly nil", func() {
			trs := TaskResultSummary{
				Key: TaskResultSummaryKey(ctx, reqKey),
			}
			So(trs.TaskAuthInfo(), ShouldEqual, acls.TaskAuthInfo{
				TaskID: "65aba3a3e6b99310",
			})
		})
	})
}

func TestPerformanceStats(t *testing.T) {
	t.Parallel()

	Convey("With Datastore", t, func() {
		ctx := memory.Use(context.Background())

		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		fullyPopulated := PerformanceStats{
			Key:                  PerformanceStatsKey(ctx, reqKey),
			BotOverheadSecs:      1,
			CacheTrim:            OperationStats{DurationSecs: 2},
			PackageInstallation:  OperationStats{DurationSecs: 3},
			NamedCachesInstall:   OperationStats{DurationSecs: 4},
			NamedCachesUninstall: OperationStats{DurationSecs: 5},
			Cleanup:              OperationStats{DurationSecs: 6},
			IsolatedDownload:     CASOperationStats{DurationSecs: 7},
			IsolatedUpload:       CASOperationStats{DurationSecs: 7},
		}
		So(datastore.Put(ctx, &fullyPopulated), ShouldBeNil)
		loaded := PerformanceStats{Key: PerformanceStatsKey(ctx, reqKey)}
		So(datastore.Get(ctx, &loaded), ShouldBeNil)
		So(loaded, ShouldResemble, fullyPopulated)
	})

	Convey("ToProto", t, func() {
		ctx := memory.Use(context.Background())

		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		Convey("ok", func() {
			ps := PerformanceStats{
				Key:                  PerformanceStatsKey(ctx, reqKey),
				BotOverheadSecs:      1,
				CacheTrim:            OperationStats{DurationSecs: 2},
				PackageInstallation:  OperationStats{DurationSecs: 3},
				NamedCachesInstall:   OperationStats{DurationSecs: 4},
				NamedCachesUninstall: OperationStats{DurationSecs: 5},
				Cleanup:              OperationStats{DurationSecs: 6},
				IsolatedDownload:     CASOperationStats{DurationSecs: 7},
				IsolatedUpload:       CASOperationStats{DurationSecs: 7},
			}
			resp, err := ps.ToProto()
			So(err, ShouldBeNil)
			So(resp, ShouldEqual, &apipb.PerformanceStats{
				BotOverhead:          float32(1),
				CacheTrim:            &apipb.OperationStats{Duration: float32(2)},
				PackageInstallation:  &apipb.OperationStats{Duration: float32(3)},
				NamedCachesInstall:   &apipb.OperationStats{Duration: float32(4)},
				NamedCachesUninstall: &apipb.OperationStats{Duration: float32(5)},
				Cleanup:              &apipb.OperationStats{Duration: float32(6)},
				IsolatedDownload:     &apipb.CASOperationStats{Duration: float32(7)},
				IsolatedUpload:       &apipb.CASOperationStats{Duration: float32(7)},
			})
		})
		Convey("error with CASOperationStats", func() {
			ps := PerformanceStats{
				Key:                  PerformanceStatsKey(ctx, reqKey),
				BotOverheadSecs:      1,
				CacheTrim:            OperationStats{DurationSecs: 2},
				PackageInstallation:  OperationStats{DurationSecs: 3},
				NamedCachesInstall:   OperationStats{DurationSecs: 4},
				NamedCachesUninstall: OperationStats{DurationSecs: 5},
				Cleanup:              OperationStats{DurationSecs: 6},
				IsolatedDownload: CASOperationStats{
					DurationSecs: 7,
					ItemsCold:    []byte("aoasfasfaiof"),
				},
				IsolatedUpload: CASOperationStats{DurationSecs: 7},
			}
			_, err := ps.ToProto()
			So(err, ShouldErrLike, "failed to get zlib reader: zlib: invalid header")
		})
	})
}

func TestOperationStats(t *testing.T) {
	t.Parallel()

	Convey("ToProto", t, func() {
		Convey("nil", func() {
			r := OperationStats{}
			So(r.ToProto(), ShouldBeNil)
		})

		Convey("ok", func() {
			r := OperationStats{DurationSecs: 1}
			So(r.ToProto(), ShouldResembleProto, apipb.OperationStats{
				Duration: 1,
			})
		})

	})
}

func TestCASOperationStats(t *testing.T) {
	t.Parallel()

	Convey("ToProto", t, func() {
		Convey("nil", func() {
			r := CASOperationStats{}
			resp, err := r.ToProto()
			So(err, ShouldBeNil)
			So(resp, ShouldBeNil)
		})

		Convey("ok", func() {
			r := CASOperationStats{
				DurationSecs: 1,
				InitialItems: 2,
				InitialSize:  3,
			}
			itemsColdBytes, err := packedintset.Pack([]int64{1, 2, 3})
			So(err, ShouldBeNil)
			itemsHotBytes, err := packedintset.Pack([]int64{4, 5, 5, 6})
			So(err, ShouldBeNil)
			r.ItemsCold = itemsColdBytes
			r.ItemsHot = itemsHotBytes
			resp, err := r.ToProto()
			So(err, ShouldBeNil)
			So(resp, ShouldResembleProto, apipb.CASOperationStats{
				Duration:            1,
				InitialNumberItems:  2,
				InitialSize:         3,
				ItemsCold:           itemsColdBytes,
				ItemsHot:            itemsHotBytes,
				NumItemsCold:        3,
				TotalBytesItemsCold: 6,
				NumItemsHot:         4,
				TotalBytesItemsHot:  20,
			})
		})

		Convey("error with Unpack used for ItemsCold and ItemsHot", func() {
			r := CASOperationStats{
				DurationSecs: 1,
				InitialItems: 2,
				InitialSize:  3,
				ItemsCold:    []byte("abcdefg"),
			}
			_, err := r.ToProto()
			So(err, ShouldErrLike, "failed to get zlib reader: zlib: invalid header")
		})
	})
}
