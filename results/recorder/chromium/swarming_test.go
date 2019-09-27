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
			buf, err := fetchOutputJSON(ctx, http.DefaultClient, swarmSvc, swarmingFake.URL, "successful-task")
			So(err, ShouldBeNil)
			So(buf, ShouldResemble, []byte("f00df00d"))
		})

		Convey(`errs if no outputs`, func() {
			_, err := fetchOutputJSON(ctx, http.DefaultClient, swarmSvc, swarmingFake.URL, "no-outputs-task")
			So(err, ShouldErrLike, "had no isolated outputs")
		})

		Convey(`errs if no output file`, func() {
			_, err := fetchOutputJSON(ctx, http.DefaultClient, swarmSvc, swarmingFake.URL, "no-output-file-task")
			So(err, ShouldErrLike, "missing expected output output.json in isolated outputs")
		})
	})

	Convey(`Getting Invocation ID`, t, func() {
		swarmingFake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path

			var resp string
			switch {
			case strings.Contains(path, "deduped-task"):
				resp = `{"deduped_from" : "parent-task", "run_id": "123410"}`
			case strings.Contains(path, "parent-task"):
				resp = `{"run_id": "abcd12"}`
			default:
				resp = `{}`
			}

			io.WriteString(w, resp)
		}))
		defer swarmingFake.Close()

		swarmSvc, err := getSwarmSvc(http.DefaultClient, swarmingFake.URL)
		So(err, ShouldBeNil)

		Convey(`for parent task`, func() {
			id, err := getInvocationId(ctx, swarmSvc, swarmingFake.URL, "parent-task")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, fmt.Sprintf("%s/abcd12", swarmingFake.URL))
		})

		Convey(`for deduped task`, func() {
			id, err := getInvocationId(ctx, swarmSvc, swarmingFake.URL, "deduped-task")
			So(err, ShouldBeNil)
			So(id, ShouldEqual, fmt.Sprintf("%s/abcd12", swarmingFake.URL))
		})
	})
}
