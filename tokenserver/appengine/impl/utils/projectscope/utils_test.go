// Copyright 2018 The LUCI Authors.
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

package projectscope

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateAccountId(t *testing.T) {

	Convey("GenerateAccountId generates compliant account ids", t, func() {
		accountID := GenerateAccountID("foo", "bar")
		So(accountID, ShouldResemble, "ps-bar--------e76278ce2f898751")
	})

	Convey("digest() regression", t, func() {
		h := digest("foo", "bar")
		So(h, ShouldResemble, [8]uint8{231, 98, 120, 206, 47, 137, 135, 81})
	})

	Convey("encode() regression", t, func() {
		e := encode("bar")
		So(e, ShouldResemble, [10]uint8{'b', 'a', 'r', 45, 45, 45, 45, 45, 45, 45})
	})
}
