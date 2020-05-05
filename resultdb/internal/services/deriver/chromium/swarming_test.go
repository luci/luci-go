// Copyright 2020 The LUCI Authors.
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

package chromium

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"

	swarmingAPI "go.chromium.org/luci/common/api/swarming/swarming/v1"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/services/deriver/chromium/formats"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSwarming(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = internal.WithHTTPClient(ctx, &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	})

	// Set up fake isolatedserver.
	isoFake := isolatedfake.New()
	isoServer := httptest.NewServer(isoFake)
	defer isoServer.Close()

	Convey(`deriveProtosForWriting correctly handles tasks`, t, func(c C) {
		sizePointer := func(n int64) *int64 { return &n }

		// Inject isolated objects.
		fileDigest := isoFake.Inject("ns", []byte(`"f00df00d"`))
		isoOut := isolated.Isolated{
			Files: map[string]isolated.File{"output.json": {Digest: fileDigest, Size: sizePointer(8)}},
		}
		isoOutBytes, err := json.Marshal(isoOut)
		So(err, ShouldBeNil)
		outputsDigest := isoFake.Inject("ns", isoOutBytes)

		badDigest := isoFake.Inject("ns", []byte("baadf00d"))
		isoOut = isolated.Isolated{
			Files: map[string]isolated.File{"artifact": {Digest: badDigest, Size: sizePointer(8)}},
		}
		isoOutBytes, err = json.Marshal(isoOut)
		So(err, ShouldBeNil)
		badOutputsDigest := isoFake.Inject("ns", isoOutBytes)

		eofDigest := isoFake.Inject("eof", []byte(``))
		isoOut = isolated.Isolated{
			Files: map[string]isolated.File{"output.json": {Digest: eofDigest, Size: sizePointer(0)}},
		}
		isoOutBytes, err = json.Marshal(isoOut)
		So(err, ShouldBeNil)
		eofOutputsDigest := isoFake.Inject("eof", isoOutBytes)

		// Set up fake swarming service.
		swarmingFake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := &swarmingAPI.SwarmingRpcsTaskResult{
				CreatedTs: "2014-09-24T13:49:16.01",
				Tags: []string{
					"bucket:bkt",
					"builder:blder",
					"test_suite:foo_unittests",
					"test_id_prefix:ninja://chrome/tests:browser_tests/",
					"stepname:foo_unittests on Mac",
				},
			}
			hasCompletion := true

			switch r.URL.Path {
			case fmt.Sprintf("/%stask/pending-task/result", swarmingAPIEndpoint):
				resp.State = "PENDING"
				resp.TaskId = "pending-task"
				hasCompletion = false

			case fmt.Sprintf("/%stask/bot-died-task/result", swarmingAPIEndpoint):
				resp.State = "BOT_DIED"

			case fmt.Sprintf("/%stask/timed-out-task/result", swarmingAPIEndpoint):
				resp.State = "TIMED_OUT"

			case fmt.Sprintf("/%stask/timed-out-outputs-task/result", swarmingAPIEndpoint):
				resp.State = "TIMED_OUT"
				resp.OutputsRef = &swarmingAPI.SwarmingRpcsFilesRef{
					Isolatedserver: isoServer.URL,
					Namespace:      "ns",
					Isolated:       string(outputsDigest),
				}

			case fmt.Sprintf("/%stask/completed-no-outputs-task/result", swarmingAPIEndpoint):
				resp.State = "COMPLETED"

			case fmt.Sprintf("/%stask/completed-outputs-hash-missing-task/result", swarmingAPIEndpoint):
				resp.State = "COMPLETED"
				resp.OutputsRef = &swarmingAPI.SwarmingRpcsFilesRef{
					Isolatedserver: isoServer.URL,
					Namespace:      "ns",
					Isolated:       fmt.Sprintf("%s%s", string(outputsDigest), string(badOutputsDigest)),
				}

			case fmt.Sprintf("/%stask/completed-no-output-file-task/result", swarmingAPIEndpoint):
				resp.State = "COMPLETED"
				resp.OutputsRef = &swarmingAPI.SwarmingRpcsFilesRef{
					Isolatedserver: isoServer.URL,
					Namespace:      "ns",
					Isolated:       string(badOutputsDigest),
				}

			case fmt.Sprintf("/%stask/completed-empty-output-file-task/result", swarmingAPIEndpoint):
				resp.State = "COMPLETED"
				resp.OutputsRef = &swarmingAPI.SwarmingRpcsFilesRef{
					Isolatedserver: isoServer.URL,
					Namespace:      "eof",
					Isolated:       string(eofOutputsDigest),
				}

			case fmt.Sprintf("/%stask/completed-outputs-task/result", swarmingAPIEndpoint):
				resp.State = "COMPLETED"
				resp.OutputsRef = &swarmingAPI.SwarmingRpcsFilesRef{
					Isolatedserver: isoServer.URL,
					Namespace:      "ns",
					Isolated:       string(outputsDigest),
				}

			case fmt.Sprintf("/%stask/no-completion-task/result", swarmingAPIEndpoint):
				resp.State = "BOT_DIED"
				hasCompletion = false

			case fmt.Sprintf("/%stask/no-test-id-prefix/result", swarmingAPIEndpoint):
				resp.State = "COMPLETED"
				resp.OutputsRef = &swarmingAPI.SwarmingRpcsFilesRef{
					Isolatedserver: isoServer.URL,
					Namespace:      "ns",
					Isolated:       string(outputsDigest),
				}
				resp.Tags = []string{
					"bucket:bkt",
					"builder:blder",
					"test_suite:foo_unittests",
					"ninja_target://chrome/tests:browser_tests",
				}

				hasCompletion = false

			default:
				resp.State = "INVALID"
			}

			if hasCompletion {
				resp.CompletedTs = "2014-09-24T14:49:16.0"
			}

			err := json.NewEncoder(w).Encode(resp)
			c.So(err, ShouldBeNil)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := GetSwarmSvc(internal.HTTPClient(ctx), swarmingFake.URL)
		So(err, ShouldBeNil)

		// Define base request we'll be using.
		swarmingHostname := strings.TrimPrefix(swarmingFake.URL, "https://")
		req := &pb.DeriveChromiumInvocationRequest{
			SwarmingTask: &pb.DeriveChromiumInvocationRequest_SwarmingTask{
				Hostname: swarmingHostname,
			},
		}

		Convey(`that are not finalized`, func() {
			task, err := swarmSvc.Task.Result("pending-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			req.SwarmingTask.Id = "pending-task"
			_, err = DeriveChromiumInvocation(task, req)
			expectedDetail := &errdetails.PreconditionFailure{
				Violations: []*errdetails.PreconditionFailure_Violation{{Type: "INCOMPLETE_CHROMIUM_SWARMING_TASK"}},
			}
			So(err, ShouldHaveAppStatus, codes.FailedPrecondition, "task pending-task is not complete yet", expectedDetail)
		})

		Convey(`that are finalized wih no outputs expected`, func() {
			task, err := swarmSvc.Task.Result("bot-died-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			inv, err := DeriveChromiumInvocation(task, req)
			So(err, ShouldBeNil)
			So(inv, ShouldNotBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)
			So(inv.Interrupted, ShouldEqual, true)
		})

		Convey(`that are finalized and may or may not contain isolated outputs`, func() {
			Convey(`and don't`, func() {
				task, err := swarmSvc.Task.Result("timed-out-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)
				So(inv.State, ShouldEqual, pb.Invocation_FINALIZED)
				So(inv.Interrupted, ShouldEqual, true)
			})

			Convey(`and do`, func() {
				task, err := swarmSvc.Task.Result("timed-out-outputs-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)

				res, err := DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
				So(len(res), ShouldEqual, 1)
				So(res[0].Expected, ShouldBeFalse)
				So(res[0].Status, ShouldEqual, pb.TestStatus_ABORT)
			})
		})

		Convey(`that are finalized and should contain outputs`, func() {
			Convey(`but don't`, func() {
				task, err := swarmSvc.Task.Result("completed-no-outputs-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)

				res, err := DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
				So(len(res), ShouldEqual, 1)
				So(res[0].TestId, ShouldEqual, "ninja://chrome/tests:browser_tests/")
				So(res[0].Expected, ShouldBeTrue)
				So(res[0].Status, ShouldEqual, pb.TestStatus_PASS)
				So(pbutil.StringPairsContain(inv.Tags, pbutil.StringPair("step_name", "foo_unittests on Mac")), ShouldBeTrue)
			})

			Convey(`and do but the hash cannot be fetched`, func() {
				task, err := swarmSvc.Task.Result("completed-outputs-hash-missing-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				ctx, cancel := context.WithTimeout(ctx, 1*time.Millisecond)
				defer cancel()

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)
				_, err = DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
			})

			Convey(`and does but outputs don't have expected file`, func() {
				task, err := swarmSvc.Task.Result("completed-no-output-file-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)
				_, err = DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
			})

			Convey(`and does but output file is empty`, func() {
				task, err := swarmSvc.Task.Result("completed-empty-output-file-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)
				_, err = DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
			})

			Convey(`and do`, func() {
				task, err := swarmSvc.Task.Result("completed-outputs-task").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)
				_, err = DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
			})

			Convey(`and do with ninja_target`, func() {
				task, err := swarmSvc.Task.Result("no-test-id-prefix").Context(ctx).Do()
				So(err, ShouldBeNil)

				inv, err := DeriveChromiumInvocation(task, req)
				So(err, ShouldBeNil)
				So(inv, ShouldNotBeNil)
				_, err = DeriveTestResults(ctx, task, req, inv)
				So(err, ShouldBeNil)
			})
		})

		Convey(`that are invalid`, func() {
			task, err := swarmSvc.Task.Result("no-completion-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			inv, err := DeriveChromiumInvocation(task, req)
			So(err, ShouldBeNil)
			So(inv, ShouldNotBeNil)
			So(inv.FinalizeTime, ShouldBeNil)
			_, err = DeriveTestResults(ctx, task, req, inv)
			So(err, ShouldBeNil)
		})
	})

	Convey(`handles Swarming errors`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case fmt.Sprintf("/%stask/200-task0/result", swarmingAPIEndpoint):
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, `{"outputs_ref": {}}`)
			case fmt.Sprintf("/%stask/5xx-task/result", swarmingAPIEndpoint):
				w.WriteHeader(http.StatusInternalServerError)
			default:
				w.WriteHeader(http.StatusNotFound)
			}

		}))
		defer swarmingFake.Close()

		swarmSvc, err := GetSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`does not error for 200`, func() {
			_, err := GetSwarmingTask(ctx, "200-task0", swarmSvc)
			So(err, ShouldBeNil)
		})

		Convey(`tags with gRPC NotFound for 404`, func() {
			_, err := GetSwarmingTask(ctx, "404-task0", swarmSvc)
			So(err, ShouldHaveAppStatus, codes.NotFound)
		})
	})

	Convey(`Getting origin task`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var resp string
			switch r.URL.Path {
			case fmt.Sprintf("/%stask/deduped-task0/result", swarmingAPIEndpoint):
				resp = `{"task_id": "deduped-task0", "deduped_from" : "first-task1", "run_id": "first-task1"}`
			case fmt.Sprintf("/%stask/first-task0/result", swarmingAPIEndpoint):
				resp = `{"task_id": "first-task0", "run_id": "first-task1"}`
			default:
				resp = `{}`
			}

			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := GetSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`for non-deduped task`, func() {
			task, err := swarmSvc.Task.Result("first-task0").Context(ctx).Do()
			So(err, ShouldBeNil)

			task, err = GetOriginTask(ctx, task, swarmSvc)
			So(err, ShouldBeNil)
			So(task.RunId, ShouldEqual, "first-task1")
			So(task.TaskId, ShouldEqual, "first-task0")
		})

		Convey(`for deduped task`, func() {
			task, err := swarmSvc.Task.Result("deduped-task0").Context(ctx).Do()
			So(err, ShouldBeNil)

			task, err = GetOriginTask(ctx, task, swarmSvc)
			So(err, ShouldBeNil)
			So(task.RunId, ShouldEqual, "first-task1")
			So(task.TaskId, ShouldEqual, "first-task0")
		})
	})

	Convey(`Converting output JSON`, t, func() {
		Convey(`chooses JSON Test Results Format correctly`, func() {
			buf := []byte(
				`{
					"version": 3,
					"tests": {
						"c1": {
							"c2": {
								"t1.html": {
									"actual": "PASS PASS PASS",
									"expected": "PASS",
									"time": 0.3,
									"times": [ 0.3, 0.2, 0.1 ]
								}
							}
						}
					}
				}`)
			inv := &pb.Invocation{}
			_, err := convertOutputJSON(ctx, inv, "", buf, nil)
			So(err, ShouldBeNil)
			So(inv, ShouldNotBeNil)
			So(inv.Tags, ShouldResembleProto, pbutil.StringPairs(formats.OriginalFormatTagKey, formats.FormatJTR))
		})

		Convey(`chooses GTest format correctly`, func() {
			buf := []byte(
				`{
					"all_tests": [ "FooTest.TestDoBar" ],
					"per_iteration_data": [{
						"FooTest.TestDoBar": [
							{
								"elapsed_time_ms": 1837,
								"losless_snippet": true,
								"output_snippet": "[ RUN      ] FooTest.TestDoBar",
								"output_snippet_base64": "WyBSVU4gICAgICBdIEZvb1Rlc3QuVGVzdERvQmFy",
								"status": "CRASH"
							}
						]
					}]
				}`)
			inv := &pb.Invocation{}
			_, err := convertOutputJSON(ctx, inv, "", buf, nil)
			So(err, ShouldBeNil)
			So(inv, ShouldNotBeNil)
			So(inv.Tags, ShouldResembleProto, pbutil.StringPairs(formats.OriginalFormatTagKey, formats.FormatGTest))
		})

		Convey(`errors on neither`, func() {
			buf := []byte(
				`{
					"all_tests": "not GTest format",
					"version": "not JSON Test Results format"
				}`)
			_, err := convertOutputJSON(ctx, &pb.Invocation{}, "", buf, nil)
			So(err, ShouldErrLike, `(and 1 other error)`)
		})
	})

	Convey(`converts swarming timestamps`, t, func() {
		Convey(`without trailing Z`, func() {
			tpb, err := convertSwarmingTs("2014-09-24T13:49:16.012345")
			So(err, ShouldBeNil)
			So(tpb, ShouldResembleProto, &tspb.Timestamp{Seconds: 1411566556, Nanos: 12345e3})
		})

		Convey(`with trailing Z`, func() {
			tpb, err := convertSwarmingTs("2014-09-24T13:49:16.012345Z")
			So(err, ShouldBeNil)
			So(tpb, ShouldResembleProto, &tspb.Timestamp{Seconds: 1411566556, Nanos: 12345e3})
		})
	})
}
