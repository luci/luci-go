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

	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

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

	Convey(`deriveProtosForWriting correctly handles tasks`, t, func() {
		// Inject isolated objects.
		fileDigest := isoFake.Inject("ns", []byte(`"f00df00d"`))
		isoOut := isolated.Isolated{
			Files: map[string]isolated.File{"output.json": {Digest: fileDigest}},
		}
		isoOutBytes, err := json.Marshal(isoOut)
		So(err, ShouldBeNil)
		outputsDigest := isoFake.Inject("ns", isoOutBytes)

		badDigest := isoFake.Inject("ns", []byte("baadf00d"))
		isoOut = isolated.Isolated{
			Files: map[string]isolated.File{"artifact": {Digest: badDigest}},
		}
		isoOutBytes, err = json.Marshal(isoOut)
		So(err, ShouldBeNil)
		badOutputsDigest := isoFake.Inject("ns", isoOutBytes)

		// Set up fake swarming service.
		swarmingFake := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := `{"state": `
			switch r.URL.Path {
			case fmt.Sprintf("/%stask/pending-task/result", swarmingAPIEndpoint):
				resp += `"PENDING"`

			case fmt.Sprintf("/%stask/bot-died-task/result", swarmingAPIEndpoint):
				resp += `"BOT_DIED"`

			case fmt.Sprintf("/%stask/timed-out-task/result", swarmingAPIEndpoint):
				resp += `"TIMED_OUT"`
			case fmt.Sprintf("/%stask/timed-out-outputs-task/result", swarmingAPIEndpoint):
				resp += `"TIMED_OUT", ` + makeOutputsRefString(isoServer.URL, outputsDigest)

			case fmt.Sprintf("/%stask/completed-no-outputs-task/result", swarmingAPIEndpoint):
				resp += fmt.Sprintf(`"COMPLETED"`)
			case fmt.Sprintf("/%stask/completed-no-output-file-task/result", swarmingAPIEndpoint):
				resp += `"COMPLETED", ` + makeOutputsRefString(isoServer.URL, badOutputsDigest)
			case fmt.Sprintf("/%stask/completed-outputs-task/result", swarmingAPIEndpoint):
				resp += `"COMPLETED", ` + makeOutputsRefString(isoServer.URL, outputsDigest)

			default:
				resp += `"INVALID"`
			}

			resp += `, "created_ts": "2014-09-24T13:49:16.01", "completed_ts": "2014-09-24T14:49:16.01"}`
			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		// Define base request we'll be using.
		swarmingHostname := strings.TrimPrefix(swarmingFake.URL, "https://")
		req := &pb.DeriveInvocationRequest{
			SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
				Hostname: swarmingHostname,
			},
			BaseTestVariant: &pb.VariantDef{Def: map[string]string{
				"bucket":     "bkt",
				"builder":    "blder",
				"test_suite": "foo_unittests",
			}},
		}

		Convey(`that are not finalized`, func() {
			req.SwarmingTask.Id = "pending-task"
			_, err := DeriveInvocation(ctx, req)
			So(err, ShouldErrLike, "unexpectedly incomplete")
		})

		Convey(`that are finalized wih no outputs expected`, func() {
			req.SwarmingTask.Id = "bot-died-task"
			inv, err := DeriveInvocation(ctx, req)
			So(err, ShouldBeNil)
			So(inv.State, ShouldEqual, pb.Invocation_INTERRUPTED)
		})

		Convey(`that are finalized and may or may not contain isolated outputs`, func() {
			Convey(`and don't`, func() {
				req.SwarmingTask.Id = "timed-out-task"
				inv, err := DeriveInvocation(ctx, req)
				So(err, ShouldBeNil)
				So(inv.State, ShouldEqual, pb.Invocation_INTERRUPTED)
			})

			Convey(`and do`, func() {
				req.SwarmingTask.Id = "timed-out-outputs-task"
				_, err := DeriveInvocation(ctx, req)
				So(err, ShouldErrLike, "cannot unmarshal string into Go value")
			})
		})

		Convey(`that are finalized and should contain outputs`, func() {
			Convey(`but don't`, func() {
				req.SwarmingTask.Id = "completed-no-outputs-task"
				_, err := DeriveInvocation(ctx, req)
				So(err, ShouldErrLike, "missing expected isolated outputs")
			})

			Convey(`and does but outputs don't have expected file`, func() {
				req.SwarmingTask.Id = "completed-no-output-file-task"
				_, err := DeriveInvocation(ctx, req)
				So(err, ShouldErrLike, "missing expected output output.json in isolated outputs")
			})

			Convey(`and do`, func() {
				req.SwarmingTask.Id = "completed-outputs-task"
				_, err := DeriveInvocation(ctx, req)
				So(err, ShouldErrLike, "cannot unmarshal string into Go value")
			})
		})
	})

	Convey(`handles Swarming errors`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case fmt.Sprintf("/%stask/200-task/result", swarmingAPIEndpoint):
				w.WriteHeader(http.StatusOK)
				io.WriteString(w, `{"outputs_ref": {}}`)
			case fmt.Sprintf("/%stask/5xx-task/result", swarmingAPIEndpoint):
				w.WriteHeader(http.StatusInternalServerError)
			default:
				w.WriteHeader(http.StatusNotFound)
			}

		}))
		defer swarmingFake.Close()

		swarmSvc, err := getSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`does not error for 200`, func() {
			_, err := getSwarmingTask(ctx, "200-task", swarmSvc)
			So(err, ShouldBeNil)
		})

		Convey(`tags with gRPC NotFound for 404`, func() {
			_, err := getSwarmingTask(ctx, "404-task", swarmSvc)
			So(err, ShouldNotBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
		})

		Convey(`tags with gRPC Internal for 5xx`, func() {
			_, err := getSwarmingTask(ctx, "5xx-task", swarmSvc)
			So(err, ShouldNotBeNil)
			So(grpcutil.Code(err), ShouldEqual, codes.Internal)
		})
	})

	Convey(`Getting origin task`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var resp string
			switch r.URL.Path {
			case fmt.Sprintf("/%stask/deduped-task/result", swarmingAPIEndpoint):
				resp = `{"task_id": "deduped-task", "deduped_from" : "first-task", "run_id": "123410"}`
			case fmt.Sprintf("/%stask/first-task/result", swarmingAPIEndpoint):
				resp = `{"task_id": "first-task", "run_id": "abcd12"}`
			default:
				resp = `{}`
			}

			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := getSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`for non-deduped task`, func() {
			task, err := swarmSvc.Task.Result("first-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			task, err = getOriginTask(ctx, task, swarmSvc)
			So(err, ShouldBeNil)
			So(task.RunId, ShouldEqual, "abcd12")
			So(task.TaskId, ShouldEqual, "first-task")
		})

		Convey(`for deduped task`, func() {
			task, err := swarmSvc.Task.Result("deduped-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			task, err = getOriginTask(ctx, task, swarmSvc)
			So(err, ShouldBeNil)
			So(task.RunId, ShouldEqual, "abcd12")
			So(task.TaskId, ShouldEqual, "first-task")
		})
	})

	Convey(`Converting output JSON`, t, func() {
		req := &pb.DeriveInvocationRequest{
			SwarmingTask: &pb.DeriveInvocationRequest_SwarmingTask{
				Hostname: "host-swarming",
				Id:       "123",
			},
			TestPathPrefix: "prefix/",
			BaseTestVariant: &pb.VariantDef{Def: map[string]string{
				"bucket":     "bkt",
				"builder":    "blder",
				"test_suite": "foo_unittests",
			}},
		}

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
			_, err := convertOutputJSON(ctx, inv, req, buf)
			So(err, ShouldBeNil)
			So(inv, ShouldNotBeNil)
			So(inv.Tags, ShouldResembleProto, pbutil.StringPairs("test_framework", "json"))
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
			_, err := convertOutputJSON(ctx, inv, req, buf)
			So(err, ShouldBeNil)
			So(inv, ShouldNotBeNil)
			So(inv.Tags, ShouldResembleProto, pbutil.StringPairs("test_framework", "gtest"))
		})

		Convey(`errors on neither`, func() {
			buf := []byte(
				`{
					"all_tests": "not GTest format",
					"version": "not JSON Test Results format"
				}`)
			_, err := convertOutputJSON(ctx, &pb.Invocation{}, req, buf)
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

// makeOutputsRefString makes the "outputs_ref" value in a swarming task result response.
func makeOutputsRefString(url string, digest isolated.HexDigest) string {
	return fmt.Sprintf(
		`"outputs_ref": {"isolatedserver": "%s", "namespace": "ns", "isolated": "%s"}`,
		url, digest)
}
