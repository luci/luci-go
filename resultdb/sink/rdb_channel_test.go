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

	sinkpb "go.chromium.org/luci/resultdb/proto/sink/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSinkArtsToRpcArts(t *testing.T) {
	t.Parallel()

	Convey("sinkArtsToRpcArts", t, func() {
		ctx := context.Background()
		sinkArts := map[string]*sinkpb.Artifact{}

		Convey("sets the size of the artifact", func() {
			Convey("with the length of contents", func() {
				sinkArts["art1"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_Contents{Contents: []byte("123")}}
				rpcArts := sinkArtsToRpcArts(ctx, sinkArts)
				So(len(rpcArts), ShouldEqual, 1)
				So(rpcArts[0].Size, ShouldEqual, 3)
			})

			Convey("with the size of the file", func() {
				sinkArts["art1"] = testArtifactWithFile(func(f *os.File) {
					n, err := f.WriteString("test artifact")
					So(err, ShouldBeNil)
					So(n, ShouldEqual, len("test artifact"))
				})
				defer os.Remove(sinkArts["art1"].GetFilePath())

				rpcArts := sinkArtsToRpcArts(ctx, sinkArts)
				So(len(rpcArts), ShouldEqual, 1)
				So(rpcArts[0].Size, ShouldEqual, len("test artifact"))
			})

			Convey("with -1 if the file is not accessible", func() {
				sinkArts["art1"] = &sinkpb.Artifact{
					Body: &sinkpb.Artifact_FilePath{FilePath: "does-not-exist/foo/bar"}}
				rpcArts := sinkArtsToRpcArts(ctx, sinkArts)
				So(len(rpcArts), ShouldEqual, 1)
				So(rpcArts[0].Size, ShouldEqual, -1)
			})
		})
	})
}
