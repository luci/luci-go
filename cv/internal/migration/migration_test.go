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

package migration

import (
	"testing"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClsOf(t *testing.T) {
	t.Parallel()

	Convey("clsOf works", t, func() {
		a := &cvbqpb.Attempt{}
		So(clsOf(a), ShouldEqual, "NO CLS")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
		}
		So(clsOf(a), ShouldEqual, "1 CLs: [abc 1/2]")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
			{Host: "abc", Change: 2, Patchset: 3},
		}
		So(clsOf(a), ShouldEqual, "2 CLs: [abc 1/2 2/3]")

		a.GerritChanges = []*cvbqpb.GerritChange{
			{Host: "abc", Change: 1, Patchset: 2},
			{Host: "xyz", Change: 2, Patchset: 3},
			{Host: "xyz", Change: 3, Patchset: 4},
			{Host: "abc", Change: 4, Patchset: 5},
			{Host: "abc", Change: 5, Patchset: 6},
		}
		So(clsOf(a), ShouldEqual, "5 CLs: [abc 1/2] [xyz 2/3 3/4] [abc 4/5 5/6]")
	})
}
