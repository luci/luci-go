// Copyright 2023 The LUCI Authors.
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

package protoutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	. "go.chromium.org/luci/common/testing/assertions"
	milopb "go.chromium.org/luci/milo/proto/v1"
)

func TestValidateConsolePredicate(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateConsolePredicate`, t, func() {
		Convey(`valid`, func() {
			err := ValidateConsolePredicate(&milopb.ConsolePredicate{
				Project: "project",
				Builder: &buildbucketpb.BuilderID{
					Project: "another_project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			})
			So(err, ShouldBeNil)
		})

		Convey(`valid without builder`, func() {
			err := ValidateConsolePredicate(&milopb.ConsolePredicate{
				Project: "project",
			})
			So(err, ShouldBeNil)
		})

		Convey(`partial builder`, func() {
			err := ValidateConsolePredicate(&milopb.ConsolePredicate{
				Project: "project",
				Builder: &buildbucketpb.BuilderID{
					Bucket:  "bucket",
					Builder: "builder",
				},
			})
			So(err, ShouldNotBeNil)
			So(err, ShouldErrLike, "builder: project must match")
		})

	})
}
