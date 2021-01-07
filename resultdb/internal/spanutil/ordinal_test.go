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

package spanutil

import (
	"testing"

	"cloud.google.com/go/spanner"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCommitPosition(t *testing.T) {
	validDomain := "gitiles://chromium.googlesource.com/chromium/src/+/refs/heads/main"
	validCP := &pb.CommitPosition{
		Host:     "chromium.googlesource.com",
		Project:  "chromium/src",
		Ref:      "refs/heads/main",
		Position: 1,
	}
	Convey(`TestCommitPosition`, t, func() {
		Convey(`ToOrdinalDomain`, func() {
			So(GitilesCommitOrdinalDomain(validCP), ShouldEqual, validDomain)
		})
		Convey(`FromOrdinal`, func() {
			cp, err := GitilesCommitFromOrdinalFields(validDomain, spanner.NullInt64{Int64: 1, Valid: true})
			So(err, ShouldBeNil)
			So(cp, ShouldResembleProto, validCP)

			cp, err = GitilesCommitFromOrdinalFields("", spanner.NullInt64{})
			So(cp, ShouldBeNil)
			So(err, ShouldBeNil)

			_, err = GitilesCommitFromOrdinalFields(validDomain, spanner.NullInt64{})
			So(err, ShouldErrLike, "ordinal field was not set")

			_, err = GitilesCommitFromOrdinalFields("http://badDomain", spanner.NullInt64{Int64: 1, Valid: true})
			So(err, ShouldErrLike, "unsupported ordinal domain")

			_, err = GitilesCommitFromOrdinalFields("", spanner.NullInt64{Int64: 1, Valid: true})
			So(err, ShouldErrLike, "ordinal 1 does not make sense without a domain")

		})
	})
}
