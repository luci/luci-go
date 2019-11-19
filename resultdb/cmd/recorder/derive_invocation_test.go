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

package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tspb "github.com/golang/protobuf/ptypes/timestamp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	"go.chromium.org/luci/resultdb/cmd/recorder/chromium/formats"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateDeriveInvocationRequest(t *testing.T) {
	Convey(`TestValidateDeriveInvocationRequest`, t, func() {
		Convey(`Valid`, func() {
			req := &pb.DeriveInvocationRequest{
				SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
					Hostname: "swarming-host.appspot.com",
					Id:       "beeff00d",
				},
			}
			So(validateDeriveInvocationRequest(req), ShouldBeNil)

			Convey(`with base_test_variant populated`, func() {
				req.BaseTestVariant = pbutil.Variant(
					"k1", "v1",
					"key/k2", "v2",
					"key/with/part/k3", "v3",
				)
				So(validateDeriveInvocationRequest(req), ShouldBeNil)
			})
		})

		Convey(`Invalid swarming_task`, func() {
			Convey(`missing`, func() {
				req := &pb.DeriveInvocationRequest{}
				So(validateDeriveInvocationRequest(req), ShouldErrLike, "swarming_task missing")
			})

			Convey(`missing hostname`, func() {
				req := &pb.DeriveInvocationRequest{
					SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
						Id: "beeff00d",
					},
				}
				So(validateDeriveInvocationRequest(req), ShouldErrLike, "swarming_task.hostname missing")
			})

			Convey(`bad hostname`, func() {
				req := &pb.DeriveInvocationRequest{
					SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
						Hostname: "https://swarming-host.appspot.com",
						Id:       "beeff00d",
					},
				}
				So(validateDeriveInvocationRequest(req), ShouldErrLike,
					"swarming_task.hostname should not have prefix")
			})

			Convey(`missing id`, func() {
				req := &pb.DeriveInvocationRequest{
					SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
						Hostname: "swarming-host.appspot.com",
					},
				}
				So(validateDeriveInvocationRequest(req), ShouldErrLike, "swarming_task.id missing")
			})
		})

		Convey(`Invalid base_test_variant`, func() {
			req := &pb.DeriveInvocationRequest{
				SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
					Hostname: "swarming-host.appspot.com",
					Id:       "beeff00d",
				},
				BaseTestVariant: pbutil.Variant("1", "b"),
			}
			So(validateDeriveInvocationRequest(req), ShouldErrLike, "key: does not match")
		})
	})
}

func TestDeriveInvocation(t *testing.T) {
	Convey(`TestDeriveInvocation`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ct := testclock.TestRecentTimeUTC

		testutil.MustApply(ctx, testutil.InsertInvocation("inserted", pb.Invocation_COMPLETED, "", ct))

		Convey(`calling to shouldWriteInvocation works`, func() {
			Convey(`if we already have the invocation written`, func() {
				doWrite, err := shouldWriteInvocation(ctx, span.Client(ctx).Single(), "inserted")
				So(err, ShouldBeNil)
				So(doWrite, ShouldBeFalse)
			})

			Convey(`if we don't yet have the invocation written`, func() {
				doWrite, err := shouldWriteInvocation(ctx, span.Client(ctx).Single(), "another")
				So(err, ShouldBeNil)
				So(doWrite, ShouldBeTrue)
			})
		})

		ctx = internal.WithHTTPClient(ctx, &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		})

		// Set up fake isolatedserver.
		isoFake := isolatedfake.New()
		isoServer := httptest.NewServer(isoFake)
		defer isoServer.Close()

		// Inject isolated objects.
		fileDigest := isoFake.Inject("ns", []byte(`{
			"all_tests": [
				"FooTest.TestDoBar",
				"FooTest.DoesBar"
			],
			"per_iteration_data": [{
				"MyInstantiation/FooTest.DoesBar/1": [
					{
						"status": "SUCCESS",
						"elapsed_time_ms": 100
					}
				],
				"FooTest.TestDoBar": [
					{
						"status": "CRASH",
						"elapsed_time_ms": 200
					},
					{
						"status": "FAILURE",
						"elapsed_time_ms": 300
					}
				]
			}]
		}`))
		isoOut := isolated.Isolated{
			Files: map[string]isolated.File{"output.json": {Digest: fileDigest}},
		}
		isoOutBytes, err := json.Marshal(isoOut)
		So(err, ShouldBeNil)
		outputsDigest := isoFake.Inject("ns", isoOutBytes)

		// Set up fake swarming service.
		// This service only supports one task and returns the same one regardless of input.
		swarmingFake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			io.WriteString(w, fmt.Sprintf(`{
				"state": "COMPLETED",
				"outputs_ref": {
					"isolatedserver": "%s",
					"namespace": "ns",
					"isolated": "%s"
				},
				"created_ts": "2019-10-14T13:49:16.01",
				"completed_ts": "2019-10-14T14:49:16.01"
			}`, isoServer.URL, outputsDigest))
		}))
		defer swarmingFake.Close()

		// Define base request we'll be using.
		swarmingHostname := strings.TrimPrefix(swarmingFake.URL, "https://")
		variant := pbutil.Variant(
			"bucket", "bkt",
			"builder", "blder",
			"test_suite", "foo_unittests",
		)
		req := &pb.DeriveInvocationRequest{
			SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
				Hostname: swarmingHostname,
			},
			BaseTestVariant: variant,
		}

		recorder := &recorderServer{}

		Convey(`inserts a new invocation`, func() {
			req.SwarmingTask.Id = "completed-task"
			inv, err := recorder.DeriveInvocation(ctx, req)
			So(err, ShouldBeNil)

			So(inv, ShouldResembleProto, &pb.Invocation{
				Name:         inv.Name, // inv.Name is non-determinisic in this test
				State:        pb.Invocation_COMPLETED,
				CreateTime:   &tspb.Timestamp{Seconds: 1571060956, Nanos: 1e7},
				Tags:         pbutil.StringPairs(formats.OriginalFormatTagKey, formats.FormatGTest),
				FinalizeTime: &tspb.Timestamp{Seconds: 1571064556, Nanos: 1e7},
				Deadline:     &tspb.Timestamp{Seconds: 1571064556, Nanos: 1e7},
			})

			// Assert we wrote correct test results.
			txn := span.Client(ctx).ReadOnlyTransaction()
			defer txn.Close()
			trs, _, err := span.QueryTestResults(ctx, txn, span.TestResultQuery{
				InvocationIDs: []span.InvocationID{span.MustParseInvocationName(inv.Name)},
				PageSize:      100,
			})
			So(err, ShouldBeNil)
			So(trs, ShouldHaveLength, 3)
			So(trs[0].TestPath, ShouldEqual, "FooTest.DoesBar")
			So(trs[0].Status, ShouldEqual, pb.TestStatus_PASS)
			So(trs[0].Variant, ShouldResembleProto, pbutil.Variant(
				"bucket", "bkt",
				"builder", "blder",
				"test_suite", "foo_unittests",
				"param/instantiation", "MyInstantiation",
				"param/id", "1",
			))
			So(trs[1].TestPath, ShouldEqual, "FooTest.TestDoBar")
			So(trs[1].Status, ShouldEqual, pb.TestStatus_CRASH)
			So(trs[2].TestPath, ShouldEqual, "FooTest.TestDoBar")
			So(trs[2].Status, ShouldEqual, pb.TestStatus_FAIL)
		})
	})
}
