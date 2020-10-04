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

package flag

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDateFlag(t *testing.T) {
	t.Parallel()

	Convey(`DateFlag`, t, func() {
		var t time.Time
		f := Date(&t)

		So(f.Set("2020-01-24"), ShouldBeNil)
		So(t, ShouldResemble, time.Date(2020, 1, 24, 0, 0, 0, 0, time.UTC))

		So(f.String(), ShouldEqual, "2020-01-24")
		So(f.Get(), ShouldResemble, t)
	})
}
