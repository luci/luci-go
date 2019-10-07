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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

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

func TestFetchingOutputJson(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	ctx = internal.WithHTTPClient(ctx, http.DefaultClient)

	Convey(`Getting output JSON file`, t, func() {
		// Set up fake isolatedserver.
		isoFake := isolatedfake.New()
		isoServer := httptest.NewServer(isoFake)
		defer isoServer.Close()

		// Inject objects.
		fileDigest := isoFake.Inject("ns", []byte("f00df00d"))
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
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			var resp string
			switch path {
			case fmt.Sprintf("/%stask/successful-task/result", swarmingAPIEndpoint):
				resp = fmt.Sprintf(
					`{"outputs_ref": {"isolatedserver": "%s", "namespace": "ns", "isolated": "%s"}}`,
					isoServer.URL, outputsDigest)
			case fmt.Sprintf("/%stask/no-output-file-task/result", swarmingAPIEndpoint):
				resp = fmt.Sprintf(
					`{"outputs_ref": {"isolatedserver": "%s", "namespace": "ns", "isolated": "%s"}}`,
					isoServer.URL, badOutputsDigest)
			default:
				resp = `{"outputs_ref": {}}`
			}

			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := getSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`successfully gets output file`, func() {
			task, err := swarmSvc.Task.Result("successful-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			buf, err := fetchOutputJSON(ctx, task)
			So(err, ShouldBeNil)
			So(buf, ShouldResemble, []byte("f00df00d"))
		})

		Convey(`errs if no outputs`, func() {
			task, err := swarmSvc.Task.Result("no-outputs-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			_, err = fetchOutputJSON(ctx, task)
			So(err, ShouldErrLike, "no isolated outputs")
		})

		Convey(`errs if no output file`, func() {
			task, err := swarmSvc.Task.Result("no-output-file-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			_, err = fetchOutputJSON(ctx, task)
			So(err, ShouldErrLike, "missing expected output output.json in isolated outputs")
		})
	})

	Convey(`handles Swarming errors`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			switch path {
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

	Convey(`Getting Invocation ID`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			var resp string
			switch path {
			case fmt.Sprintf("/%stask/deduped-task/result", swarmingAPIEndpoint):
				resp = `{"deduped_from" : "first-task", "run_id": "123410"}`
			case fmt.Sprintf("/%stask/first-task/result", swarmingAPIEndpoint):
				resp = `{"run_id": "abcd12"}`
			default:
				resp = `{}`
			}

			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := getSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`for non-deduped task`, func() {
			req.SwarmingTask.Id = "first-task"
			task, err := swarmSvc.Task.Result(req.SwarmingTask.Id).Context(ctx).Do()
			So(err, ShouldBeNil)

			id, err := getInvocationID(ctx, task, swarmSvc, req)
			So(err, ShouldBeNil)
			So(id, ShouldEqual,
				"from_swarming_1273df315bb3d4933c67a1bdc23164d6bb17222b5ff487cf1ce6aa4e7c6369da")
		})

		Convey(`for deduped task`, func() {
			req.SwarmingTask.Id = "deduped-task"
			task, err := swarmSvc.Task.Result(req.SwarmingTask.Id).Context(ctx).Do()
			So(err, ShouldBeNil)

			id, err := getInvocationID(ctx, task, swarmSvc, req)
			So(err, ShouldBeNil)
			So(id, ShouldEqual,
				"from_swarming_1273df315bb3d4933c67a1bdc23164d6bb17222b5ff487cf1ce6aa4e7c6369da")
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
			inv, _, err := convertOutputJSON(ctx, req, buf)
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
			inv, _, err := convertOutputJSON(ctx, req, buf)
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
			_, _, err := convertOutputJSON(ctx, req, buf)
			So(err, ShouldErrLike, `(and 1 other error)`)
		})
	})
}
