// Copyright 2017 The LUCI Authors.
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

package access

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAction(t *testing.T) {
	Convey(`Test marshalling and unmarshalling`, t, func() {
		result, err := UnmarshalAction(MarshalAction(AccessBucket))
		So(err, ShouldBeNil)
		So(result, ShouldEqual, AccessBucket)

		value := Action(ViewBuild|SearchBuilds|AccessBucket)
		result, err = UnmarshalAction(MarshalAction(value))
		So(err, ShouldBeNil)
		So(result, ShouldEqual, value)
	})
}
