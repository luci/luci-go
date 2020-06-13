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

package sink

import (
	"context"
	"os"
	"testing"

	"go.chromium.org/luci/common/sync/dispatcher/buffer"
	sinkpb "go.chromium.org/luci/resultdb/sink/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUploadArtifacts(t *testing.T) {
	t.Parallel()

	Convey("uploadArtifacts", t, func() {
		ctx := context.Background()
		tr, cleanup := validTestResult()
		defer cleanup()

		ua := make(chan *uploadTask)
		ac := &artifactChannel{
			cfg: testServerConfig(nil, "127.0.0.1:123", "secret"),
			testUpload: func(ctx context.Context, b *buffer.Batch) error {
				for _, d := range b.Data {
					ua <- d.(*uploadTask)
				}
				return nil
			},
		}
		So(ac.init(ctx), ShouldBeNil)

		Convey("generates artifact name", func() {
			ac.cfg.Invocation = "invocations/u:test-inv"
			// test-id must be url encoded
			tr.TestId = "test 123"
			tr.ResultId = "rid"
			tr.Artifacts["art1"] = &sinkpb.Artifact{
				Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}

			ac.scheduleUploads(tr)
			go ac.closeAndDrain(ctx)
			uploaded := <-ua
			So(uploaded.artifactName, ShouldEqual,
				"invocations/u:test-inv/tests/test%20123/result/rid/artifact/art1")
		})

		Convey("sets contents with size", func() {
			tr.Artifacts["art1"] = &sinkpb.Artifact{
				Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}

			ac.scheduleUploads(tr)
			go ac.closeAndDrain(ctx)
			uploaded := <-ua
			So(uploaded.contents, ShouldResemble, []byte("123"))
		})

		Convey("sets filePath with size", func() {
			contents := "test artifact"
			tr.Artifacts["art1"] = testArtifactWithFile(func(f *os.File) {
				_, err := f.WriteString(contents)
				So(err, ShouldBeNil)
			})
			fp := tr.Artifacts["art1"].GetFilePath()
			defer os.Remove(fp)

			ac.scheduleUploads(tr)
			go ac.closeAndDrain(ctx)
			uploaded := <-ua
			So(uploaded.filePath, ShouldEqual, fp)
		})
	})
}
