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
	Convey(`Getting output JSON file`, t, func() {
		ctx := context.Background()

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

		Convey(`successfully gets output file`, func() {
			buf, err := fetchOutputJSON(ctx, swarmingFake.URL, "successful-task")
			So(err, ShouldBeNil)
			So(buf, ShouldResemble, []byte("f00df00d"))
		})

		Convey(`errs if no outputs`, func() {
			_, err := fetchOutputJSON(ctx, swarmingFake.URL, "no-outputs-task")
			So(err, ShouldErrLike, "had no isolated outputs")
		})

		Convey(`errs if no output file`, func() {
			_, err := fetchOutputJSON(ctx, swarmingFake.URL, "no-output-file-task")
			So(err, ShouldErrLike, "missing expected output output.json in isolated outputs")
		})
	})
}
