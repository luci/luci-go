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
	"strings"
	"testing"

	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient/isolatedfake"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/recorder/spanner"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchingOutputJson(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

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
			switch {
			case strings.Contains(path, "successful-task"):
				resp = fmt.Sprintf(
					`{"outputs_ref": {"isolatedserver": "%s", "namespace": "ns", "isolated": "%s"}}`,
					isoServer.URL, outputsDigest)
			case strings.Contains(path, "no-output-file-task"):
				resp = fmt.Sprintf(
					`{"outputs_ref": {"isolatedserver": "%s", "namespace": "ns", "isolated": "%s"}}`,
					isoServer.URL, badOutputsDigest)
			default:
				resp = fmt.Sprintf(`{"outputs_ref": {}}`)
			}

			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := getSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`successfully gets output file`, func() {
			task, err := swarmSvc.Task.Result("successful-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			buf, err := fetchOutputJSON(ctx, task, http.DefaultClient)
			So(err, ShouldBeNil)
			So(buf, ShouldResemble, []byte("f00df00d"))
		})

		Convey(`errs if no outputs`, func() {
			task, err := swarmSvc.Task.Result("no-outputs-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			_, err = fetchOutputJSON(ctx, task, http.DefaultClient)
			So(err, ShouldErrLike, "no isolated outputs")
		})

		Convey(`errs if no output file`, func() {
			task, err := swarmSvc.Task.Result("no-output-file-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			_, err = fetchOutputJSON(ctx, task, http.DefaultClient)
			So(err, ShouldErrLike, "missing expected output output.json in isolated outputs")
		})
	})

	Convey(`Getting Invocation ID`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			var resp string
			switch {
			case strings.Contains(path, "deduped-task"):
				resp = `{"deduped_from" : "first-task", "run_id": "123410"}`
			case strings.Contains(path, "first-task"):
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
			task, err := swarmSvc.Task.Result("first-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			id, err := getInvocationId(ctx, task, swarmSvc, swarmingFake.URL)
			So(err, ShouldBeNil)
			So(id, ShouldEqual, fmt.Sprintf("%s/abcd12", swarmingFake.URL))
		})

		Convey(`for deduped task`, func() {
			task, err := swarmSvc.Task.Result("deduped-task").Context(ctx).Do()
			So(err, ShouldBeNil)

			id, err := getInvocationId(ctx, task, swarmSvc, swarmingFake.URL)
			So(err, ShouldBeNil)
			So(id, ShouldEqual, fmt.Sprintf("%s/abcd12", swarmingFake.URL))
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
			inv, err := convertOutputJSON(ctx, req, &spanner.Client{}, "invID", buf)
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
			inv, err := convertOutputJSON(ctx, req, &spanner.Client{}, "invID", buf)
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
			_, err := convertOutputJSON(ctx, req, &spanner.Client{}, "invID", buf)
			So(err, ShouldErrLike, `(and 1 other error)`)
		})
	})
}
