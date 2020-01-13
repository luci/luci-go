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

package internal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseRealm(t *testing.T) {
	Convey(`success`, t, func() {
		luciProject, realmID, err := ParseRealm("chromium/public")
		So(luciProject, ShouldEqual, "chromium")
		So(realmID, ShouldEqual, "public")
		So(err, ShouldBeNil)
	})

	Convey(`realm malformatted`, t, func() {
		luciProject, realmID, err := ParseRealm("wrong")
		So(luciProject, ShouldEqual, "")
		So(realmID, ShouldEqual, "")
		So(err, ShouldErrLike, `realm "wrong" is malformatted`)
	})
}
