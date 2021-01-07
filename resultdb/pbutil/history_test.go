// Copyright 2021 The LUCI Authors.
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

package pbutil

import (
	"testing"

	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateCommitPosition(t *testing.T) {
	t.Parallel()
	Convey(`TestValidateCommitPosition`, t, func() {
		param := &pb.CommitPosition{
			Host:     "chromium.googlesource.com",
			Project:  "chromium/src",
			Ref:      "refs/heads/master",
			Position: 123456,
		}
		Convey(`valid`, func() {
			So(ValidateCommitPosition(param), ShouldBeNil)
		})
		Convey(`no host`, func() {
			param.Host = ""
			err := ValidateCommitPosition(param)
			So(err, ShouldErrLike, `host is required`)
		})
		Convey(`no project`, func() {
			param.Project = ""
			err := ValidateCommitPosition(param)
			So(err, ShouldErrLike, `project is required`)
		})
		Convey(`no ref`, func() {
			param.Ref = ""
			err := ValidateCommitPosition(param)
			So(err, ShouldErrLike, `ref must match refs/.*`)
		})
		Convey(`bad ref`, func() {
			param.Ref = "master"
			err := ValidateCommitPosition(param)
			So(err, ShouldErrLike, `ref must match refs/.*`)
		})
		Convey(`no position`, func() {
			param.Position = 0
			err := ValidateCommitPosition(param)
			So(err, ShouldErrLike, `position is required`)
		})
	})
}
