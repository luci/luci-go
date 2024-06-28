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
	"bytes"
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/klauspost/compress/zlib"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/data/packedintset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cursor/cursorpb"

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

func TestTaskResultSummary(t *testing.T) {
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

		Convey("Can round trip", func() {
			So(datastore.Put(ctx, &fullyPopulated), ShouldBeNil)
			loaded := TaskResultSummary{Key: fullyPopulated.Key}
			So(datastore.Get(ctx, &loaded), ShouldBeNil)
			So(loaded, ShouldResemble, fullyPopulated)
		})

		Convey("ToProto", func() {
			So(fullyPopulated.ToProto(), ShouldResembleProto, &apipb.TaskResultResponse{
				BotDimensions: []*apipb.StringListPair{
					{Key: "cpu", Value: []string{"x86_64"}},
					{Key: "os", Value: []string{"linux"}},
				},
				BotId:               "bot123",
				BotIdleSinceTs:      timestamppb.New(testTime.Add(-30 * time.Minute)),
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

		Convey("ToProto: mostly empty", func() {
			trs := TaskResultSummary{
				TaskResultCommon: TaskResultCommon{
					Modified: testTime,
				},
				Created: testTime.Add(-2 * time.Hour),
				Key:     TaskResultSummaryKey(ctx, reqKey),
			}
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
				TaskResultCommon: TaskResultCommon{
					State: apipb.TaskState_COMPLETED,
				},
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

			statsKey := trs.PerformanceStatsKey(ctx)
			So(statsKey, ShouldNotBeNil)
			stats := &PerformanceStats{Key: statsKey}
			So(datastore.Get(ctx, stats), ShouldBeNil)

			resp, err := stats.ToProto()
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
	})

	Convey("TaskAuthInfo", t, func() {
		ctx := memory.Use(context.Background())
		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		Convey("Fresh enough entity", func() {
			trs := TaskResultSummary{
				Key:                  TaskResultSummaryKey(ctx, reqKey),
				RequestAuthenticated: "authenticated-user@example.com",
				RequestRealm:         "example-realm",
				RequestPool:          "example-pool",
				RequestBotID:         "bot123",
			}

			info, err := trs.TaskAuthInfo(ctx)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, &acls.TaskAuthInfo{
				TaskID:    "65aba3a3e6b99310",
				Realm:     "example-realm",
				Pool:      "example-pool",
				BotID:     "bot123",
				Submitter: "authenticated-user@example.com",
			})
		})

		Convey("Old entity", func() {
			trs := TaskResultSummary{
				Key: TaskResultSummaryKey(ctx, reqKey),
			}

			So(datastore.Put(ctx, &TaskRequest{
				Key:           reqKey,
				Realm:         "request-realm",
				Authenticated: "request-user@example.com",
				TaskSlices: []TaskSlice{
					{
						Properties: TaskProperties{
							Dimensions: TaskDimensions{
								"pool": {"request-pool"},
							},
						},
					},
				},
			}), ShouldBeNil)

			info, err := trs.TaskAuthInfo(ctx)
			So(err, ShouldBeNil)
			So(info, ShouldResemble, &acls.TaskAuthInfo{
				TaskID:    "65aba3a3e6b99310",
				Realm:     "request-realm",
				Pool:      "request-pool",
				Submitter: "request-user@example.com",
			})
		})
	})

	Convey("GetOutput", t, func() {
		ctx := memory.Use(context.Background())
		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		// Generate non-repeating text, to make sure that offsets are respected
		// precisely (no off by one errors) when reading chunks.
		var expectedOutput bytes.Buffer
		var chunkIndex int64

		writeChunk := func(size int, store bool) {
			if !store {
				expectedOutput.Write(emptyChunk)
				chunkIndex++
				return
			}

			buf := make([]byte, size)
			if _, err := rand.Read(buf); err != nil {
				panic(err)
			}
			expectedOutput.Write(buf)

			var compressed bytes.Buffer
			w := zlib.NewWriter(&compressed)
			if _, err := w.Write(buf); err != nil {
				panic(err)
			}
			if err := w.Close(); err != nil {
				panic(err)
			}

			err := datastore.Put(ctx, &TaskOutputChunk{
				Key:   TaskOutputChunkKey(ctx, reqKey, chunkIndex),
				Chunk: compressed.Bytes(),
			})
			if err != nil {
				panic(err)
			}
			chunkIndex++
		}

		const unfinishedSize = 1000

		// A bunch of complete chunks, one missing chunk and one unfinished chunk.
		writeChunk(ChunkSize, true)
		writeChunk(ChunkSize, true)
		writeChunk(ChunkSize, false) // missing in the datastore
		writeChunk(ChunkSize, true)
		writeChunk(unfinishedSize, true) // incomplete, being written now

		const totalSize = ChunkSize*4 + unfinishedSize
		So(expectedOutput.Len(), ShouldEqual, totalSize)

		trs := TaskResultSummary{
			TaskResultCommon: TaskResultCommon{StdoutChunks: chunkIndex},
			Key:              TaskResultSummaryKey(ctx, reqKey),
			TryNumber:        datastore.NewIndexedNullable(int64(1)),
		}

		assertExpectedOutput := func(offset, length, expectedLen int) {
			var expected []byte
			all := expectedOutput.Bytes()
			expected = all[offset:min(offset+length, len(all))]
			So(len(expected), ShouldEqual, expectedLen)

			got, err := trs.GetOutput(ctx, int64(offset), int64(length))
			So(err, ShouldBeNil)
			So(len(got), ShouldEqual, expectedLen)
			So(got, ShouldResemble, expected)
		}

		Convey("No offset", func() {
			// Reading a part of the first chunk.
			assertExpectedOutput(0, 100, 100)
			// Reading one chunk precisely.
			assertExpectedOutput(0, ChunkSize, ChunkSize)
			// Reading one chunk and a little more.
			assertExpectedOutput(0, ChunkSize+1, ChunkSize+1)
			// Reading one chunk and a lot more.
			assertExpectedOutput(0, 2*ChunkSize-1, 2*ChunkSize-1)
			// Reading two chunks precisely.
			assertExpectedOutput(0, ChunkSize*2, ChunkSize*2)
			// Reading two chunks and a bit of a missing chunk.
			assertExpectedOutput(0, ChunkSize*2+100, ChunkSize*2+100)
			// Reading 4 chunks, with the missing one in the middle.
			assertExpectedOutput(0, ChunkSize*3+100, ChunkSize*3+100)
			// Reading all available output.
			assertExpectedOutput(0, ChunkSize*5, totalSize)
		})

		Convey("With offset within the first chunk", func() {
			// Reading a part of the first chunk.
			assertExpectedOutput(200, 100, 100)
			// Reading one chunk precisely.
			assertExpectedOutput(200, ChunkSize-200, ChunkSize-200)
			// Reading two chunks.
			assertExpectedOutput(200, ChunkSize, ChunkSize)
			// Reading all available output.
			assertExpectedOutput(200, ChunkSize*5, totalSize-200)
		})

		Convey("With offset within non-first chunk", func() {
			// Reading a part of the chunk.
			assertExpectedOutput(2*ChunkSize+200, 100, 100)
			// Reading one chunk precisely.
			assertExpectedOutput(2*ChunkSize+200, ChunkSize-200, ChunkSize-200)
			// Reading two chunk.
			assertExpectedOutput(2*ChunkSize+200, ChunkSize, ChunkSize)
			// Reading all available output.
			assertExpectedOutput(2*ChunkSize+200, ChunkSize*5, totalSize-2*ChunkSize-200)
		})

		Convey("With offset in the last chunk", func() {
			// Reading a part of the last chunk
			assertExpectedOutput(4*ChunkSize+100, 100, 100)
			// Reading all available data in the last chunk.
			assertExpectedOutput(4*ChunkSize+100, ChunkSize, unfinishedSize-100)
			// Reading the last available byte.
			assertExpectedOutput(totalSize-1, ChunkSize, 1)
		})

		Convey("With offset outside of available range", func() {
			// Precisely after the last byte.
			got, err := trs.GetOutput(ctx, 4*ChunkSize+unfinishedSize, 10000)
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 0)

			// Pointing to an incomplete portion of the last chunk.
			got, err = trs.GetOutput(ctx, 4*ChunkSize+unfinishedSize+100, 10000)
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 0)

			// Outside of the last chunk entirely.
			got, err = trs.GetOutput(ctx, 5*ChunkSize+1, 10000)
			So(err, ShouldBeNil)
			So(got, ShouldHaveLength, 0)
		})
	})
}

func TestTaskRunResult(t *testing.T) {
	t.Parallel()
	var testTime = time.Date(2023, time.January, 1, 2, 3, 4, 0, time.UTC)

	Convey("With Datastore", t, func() {
		ctx := memory.Use(context.Background())

		reqKey, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		fullyPopulated := TaskRunResult{
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
			Key:            TaskRunResultKey(ctx, reqKey),
			RequestCreated: testTime,
			RequestTags:    []string{"a:b", "c:d"},
			RequestName:    "request-name",
			RequestUser:    "request-user",
			BotID:          "some-bot-id",
			CostUSD:        123.456,
			Killing:        true,
			DeadAfter:      datastore.NewUnindexedOptional(testTime.Add(time.Hour)),
		}

		Convey("Can round-trip", func() {
			So(datastore.Put(ctx, &fullyPopulated), ShouldBeNil)
			loaded := TaskRunResult{Key: fullyPopulated.Key}
			So(datastore.Get(ctx, &loaded), ShouldBeNil)
			So(loaded, ShouldResemble, fullyPopulated)
		})

		Convey("ToProto", func() {
			So(fullyPopulated.ToProto(), ShouldResembleProto, &apipb.TaskResultResponse{
				BotDimensions: []*apipb.StringListPair{
					{Key: "cpu", Value: []string{"x86_64"}},
					{Key: "os", Value: []string{"linux"}},
				},
				BotId:               "some-bot-id",
				BotIdleSinceTs:      timestamppb.New(testTime.Add(-30 * time.Minute)),
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
				CostsUsd:         []float32{123.456},
				CreatedTs:        timestamppb.New(testTime),
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
				Name:       "request-name",
				ResultdbInfo: &apipb.ResultDBInfo{
					Hostname:   "results.api.example.dev",
					Invocation: "inv123",
				},
				RunId:          "65aba3a3e6b99311",
				ServerVersions: []string{"v1.0"},
				StartedTs:      timestamppb.New(testTime.Add(-1 * time.Hour)),
				State:          apipb.TaskState_COMPLETED,
				Tags:           []string{"a:b", "c:d"},
				TaskId:         "65aba3a3e6b99310",
				User:           "request-user",
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

func TestTaskResultSummaryQueries(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	testTime := time.Date(2023, 1, 1, 2, 3, 4, 0, time.UTC)

	Convey("FilterTasksByCreationTime: ok", t, func() {
		q, err := FilterTasksByCreationTime(ctx,
			TaskResultSummaryQuery(),
			testTime,
			testTime.Add(1*time.Hour),
			nil,
		)
		So(err, ShouldBeNil)
		fq, err := q.Finalize()
		So(err, ShouldBeNil)
		So(fq.GQL(), ShouldEqual,
			"SELECT * FROM `TaskResultSummary` WHERE "+
				"`__key__` > KEY(DATASET(\"dev~app\"), \"TaskRequest\", 8793206122828791806, \"TaskResultSummary\", 1) AND "+
				"`__key__` <= KEY(DATASET(\"dev~app\"), \"TaskRequest\", 8793209897702391806, \"TaskResultSummary\", 1) "+
				"ORDER BY `__key__`",
		)
	})

	Convey("FilterTasksByCreationTime: open end", t, func() {
		q, err := FilterTasksByCreationTime(ctx,
			TaskResultSummaryQuery(),
			testTime,
			time.Time{},
			nil,
		)
		So(err, ShouldBeNil)
		fq, err := q.Finalize()
		So(err, ShouldBeNil)
		So(fq.GQL(), ShouldEqual,
			"SELECT * FROM `TaskResultSummary` WHERE "+
				"`__key__` <= KEY(DATASET(\"dev~app\"), \"TaskRequest\", 8793209897702391806, \"TaskResultSummary\", 1) "+
				"ORDER BY `__key__`",
		)
	})

	Convey("FilterTasksByCreationTime: bad time", t, func() {
		_, err := FilterTasksByCreationTime(ctx,
			TaskResultSummaryQuery(),
			time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Time{},
			nil,
		)
		So(err, ShouldErrLike, "invalid start time")
	})

	Convey("FilterTasksByCreationTime: with cursor", t, func() {
		q, err := FilterTasksByCreationTime(ctx,
			TaskResultSummaryQuery(),
			testTime,
			testTime.Add(1*time.Hour),
			&cursorpb.TasksCursor{
				LastTaskRequestEntityId: 8793206122828796666, // larger than the original end age
			},
		)
		So(err, ShouldBeNil)
		fq, err := q.Finalize()
		So(err, ShouldBeNil)
		So(fq.GQL(), ShouldEqual,
			"SELECT * FROM `TaskResultSummary` WHERE "+
				"`__key__` > KEY(DATASET(\"dev~app\"), \"TaskRequest\", 8793206122828796666, \"TaskResultSummary\", 1) AND "+
				"`__key__` <= KEY(DATASET(\"dev~app\"), \"TaskRequest\", 8793209897702391806, \"TaskResultSummary\", 1) "+
				"ORDER BY `__key__`",
		)
	})

	Convey("FilterTasksByCreationTime: with cursor past the start time", t, func() {
		_, err := FilterTasksByCreationTime(ctx,
			TaskResultSummaryQuery(),
			testTime,
			testTime.Add(1*time.Hour),
			&cursorpb.TasksCursor{
				LastTaskRequestEntityId: 8793209897702391807, // larger than the start age
			},
		)
		So(err, ShouldErrLike, "the cursor is outside of the requested time range")
	})

	Convey("FilterTasksByCreationTime: many tasks in a millisecond", t, func() {
		put := func(when time.Time, sfx int64) {
			reqKey, err := TimestampToRequestKey(ctx, when, sfx)
			So(err, ShouldBeNil)
			So(datastore.Put(ctx, &TaskResultSummary{
				Key:     TaskResultSummaryKey(ctx, reqKey),
				Created: when,
			}), ShouldBeNil)
		}

		// Create a bunch of tasks, all within the same millisecond.
		put(testTime, 0)
		put(testTime, 1)
		put(testTime, 2)
		put(testTime, 3)
		put(testTime, 4)
		put(testTime, 0xffff) // the last allowed suffix

		// The previous millisecond should be ignored.
		put(testTime.Add(-time.Millisecond), 0)
		put(testTime.Add(-time.Millisecond), 0xffff)
		// The next millisecond should be ignored.
		put(testTime.Add(time.Millisecond), 0)
		put(testTime.Add(time.Millisecond), 0xffff)

		datastore.GetTestable(ctx).CatchupIndexes()

		// Query tasks that happened within the testTime millisecond.
		query := func(lastSeenID int64, limit int) []int64 {
			var cur *cursorpb.TasksCursor
			if lastSeenID != 0 {
				cur = &cursorpb.TasksCursor{LastTaskRequestEntityId: lastSeenID}
			}
			q, err := FilterTasksByCreationTime(ctx,
				TaskResultSummaryQuery(),
				testTime,
				testTime.Add(time.Millisecond),
				cur,
			)
			if errors.Is(err, datastore.ErrNullQuery) {
				return nil
			}
			So(err, ShouldBeNil)
			var out []int64
			err = datastore.Run(ctx, q, func(e *TaskResultSummary) error {
				So(e.Created.Equal(testTime), ShouldBeTrue)
				out = append(out, e.TaskRequestKey().IntID())
				if len(out) == limit {
					return datastore.Stop
				}
				return nil
			})
			So(err, ShouldBeNil)
			return out
		}

		// Entity IDs => suffixes passed to TimestampToRequestKey.
		suffixes := func(ids []int64) (out []int) {
			for _, id := range ids {
				id = id ^ taskRequestIDMask
				out = append(out, int(id>>4)&0xffff)
			}
			return
		}

		// Querying all tasks at once. Note that tasks scheduled within the same
		// millisecond are not ordered (that are ordered by an ID suffix, which is
		// random in prod, but deterministic in this test).
		So(suffixes(query(0, 100)), ShouldResemble, []int{0xffff, 4, 3, 2, 1, 0})

		// Querying with a cursor.
		q := query(0, 2)
		So(suffixes(q), ShouldResemble, []int{0xffff, 4})
		q = query(q[1], 2)
		So(suffixes(q), ShouldResemble, []int{3, 2})
		q = query(q[1], 2)
		So(suffixes(q), ShouldResemble, []int{1, 0})

		// This was the last page, but callers don't really know that yet (because
		// the listing ends exactly on the page boundary). When querying the next
		// page, we should get no results: this signals to callers the end of the
		// listing.
		q = query(q[1], 2)
		So(suffixes(q), ShouldResemble, []int(nil))
	})

	Convey("FilterTasksByState: running", t, func() {
		qs, mode := FilterTasksByState(TaskResultSummaryQuery(), apipb.StateQuery_QUERY_RUNNING, SplitOptimally)
		So(qs, ShouldHaveLength, 1)
		So(mode, ShouldEqual, SplitOptimally)
		fq, err := qs[0].Finalize()
		So(err, ShouldBeNil)
		So(fq.GQL(), ShouldEqual,
			"SELECT * FROM `TaskResultSummary` WHERE `state` = 16 ORDER BY `__key__`")
	})

	Convey("FilterTasksByState: pending+running", t, func() {
		Convey("SplitOptimally", func() {
			qs, mode := FilterTasksByState(TaskResultSummaryQuery(), apipb.StateQuery_QUERY_PENDING_RUNNING, SplitOptimally)
			So(qs, ShouldHaveLength, 1)
			So(mode, ShouldEqual, SplitCompletely)
			fq, err := qs[0].Finalize()
			So(err, ShouldBeNil)
			So(fq.GQL(), ShouldEqual,
				"SELECT * FROM `TaskResultSummary` WHERE `state` IN ARRAY(16, 32) ORDER BY `__key__`")
		})

		Convey("SplitCompletely", func() {
			qs, mode := FilterTasksByState(TaskResultSummaryQuery(), apipb.StateQuery_QUERY_PENDING_RUNNING, SplitCompletely)
			So(qs, ShouldHaveLength, 2)
			So(mode, ShouldEqual, SplitCompletely)

			fq0, err := qs[0].Finalize()
			So(err, ShouldBeNil)
			So(fq0.GQL(), ShouldEqual,
				"SELECT * FROM `TaskResultSummary` WHERE `state` = 16 ORDER BY `__key__`")

			fq1, err := qs[1].Finalize()
			So(err, ShouldBeNil)
			So(fq1.GQL(), ShouldEqual,
				"SELECT * FROM `TaskResultSummary` WHERE `state` = 32 ORDER BY `__key__`")
		})
	})

	Convey("FilterTasksByTags", t, func() {
		tags := []*apipb.StringPair{
			{Key: "pool", Value: "chromium.tests"},
			{Key: "buildbucket_id", Value: "1"},
			{Key: "os", Value: "ubuntu1|ubuntu2"},
			{Key: "board", Value: "board1|board2"},
		}
		filter, err := NewFilter(tags)
		So(err, ShouldBeNil)

		queries := FilterTasksByTags(TaskResultSummaryQuery(), SplitOptimally, filter)
		So(len(queries), ShouldEqual, 2)

		q1, err := queries[0].Finalize()
		So(err, ShouldBeNil)
		So(q1.GQL(), ShouldEqual,
			"SELECT * FROM `TaskResultSummary` "+
				"WHERE "+
				"`tags` = \"board:board1\" AND "+
				"`tags` = \"buildbucket_id:1\" AND "+
				"`tags` = \"pool:chromium.tests\" AND "+
				"`tags` IN ARRAY(\"os:ubuntu1\", \"os:ubuntu2\") "+
				"ORDER BY `__key__`",
		)

		q2, err := queries[1].Finalize()
		So(err, ShouldBeNil)
		So(q2.GQL(), ShouldEqual,
			"SELECT * FROM `TaskResultSummary` "+
				"WHERE "+
				"`tags` = \"board:board2\" AND "+
				"`tags` = \"buildbucket_id:1\" AND "+
				"`tags` = \"pool:chromium.tests\" AND "+
				"`tags` IN ARRAY(\"os:ubuntu1\", \"os:ubuntu2\") "+
				"ORDER BY `__key__`",
		)
	})
}
