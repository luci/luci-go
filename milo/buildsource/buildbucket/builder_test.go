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

package buildbucket

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/milo/frontend/ui"
)

func TestExcludeBuilds(t *testing.T) {
	t.Parallel()

	build := func(id int64) *ui.Build {
		return &ui.Build{
			Build: &buildbucketpb.Build{Id: id},
		}
	}
	Convey(`ExcludeBuilds`, t, func() {

		excludeList := []*ui.Build{build(1), build(2)}
		excludeSet := map[int64]struct{}{}
		addBuildIDs(excludeSet, excludeList)

		origList := []*ui.Build{build(2), build(3)}
		actual := excludeBuilds(origList, excludeSet)
		So(actual, ShouldHaveLength, 1)
		So(actual[0].Id, ShouldEqual, 3)
	})
}
