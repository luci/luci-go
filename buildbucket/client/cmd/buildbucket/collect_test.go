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
	"io/ioutil"
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
)

func TestCollect(t *testing.T) {
	t.Parallel()

	Convey("Collect", t, func() {
		Convey("writeBuildDetails", func() {
			file, err := ioutil.TempFile("", "builds")
			So(err, ShouldBeNil)
			file.Close()
			defer os.Remove(file.Name())

			writeBuildDetails([]*buildbucketpb.Build{
				{Id: 123},
				{Id: 456},
			}, file.Name())

			buf, err := ioutil.ReadFile(file.Name())
			So(err, ShouldBeNil)
			So(string(buf), ShouldEqual, `[{"id":"123"},{"id":"456"}]`)
		})

		Convey("collectBuildDetails", func() {
			// TODO: figure out how to create fakeClient and test the following cases:
			//  - all builds are ended from the start, time.Sleep should not be called
			//  - one build is not ended, ended after retry
			//  - batch request is correctly formed
			//ctx := context.Background()
			//collectBuildDetails(ctx, fakeClient, []int64{123, 456}, 0)
		})
	})
}
