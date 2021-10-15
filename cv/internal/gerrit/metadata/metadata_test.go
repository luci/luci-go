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

package metadata

import (
	"strings"
	"testing"

	"go.chromium.org/luci/cv/internal/changelist"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExtract(t *testing.T) {
	t.Parallel()

	Convey("Extract works", t, func() {

		Convey("Empty", func() {
			So(Extract(`Title.`), ShouldResembleProto, []*changelist.StringPair{})
		})

		Convey("Git-style", func() {
			So(Extract(strings.TrimSpace(`
Title.

Footer: value
No-rma-lIzes: but Only key.
`)), ShouldResembleProto, []*changelist.StringPair{
				{Key: "Footer", Value: "value"},
				{Key: "No-Rma-Lizes", Value: "but Only key."},
			})
		})

		Convey("TAGS=sTyLe", func() {
			So(Extract(strings.TrimSpace(`
TAG=can BE anywhere
`)), ShouldResembleProto, []*changelist.StringPair{
				{Key: "TAG", Value: "can BE anywhere"},
			})
		})

		Convey("Ignores incorrect tags and footers", func() {
			So(Extract(strings.TrimSpace(`
Tag=must have UPPEPCASE_KEY.

Footers: must reside in the last paragraph, not above it.

Footer-key must-have-not-spaces: but this one does.
`)), ShouldResembleProto, []*changelist.StringPair{})
		})

		Convey("Sorts by keys only, keeps values ordered from last to first", func() {
			So(Extract(strings.TrimSpace(`
TAG=first
TAG=second

Footer: A
TAG=third
Footer: D
TAG=fourth
Footer: B
`)), ShouldResembleProto, []*changelist.StringPair{
				{Key: "Footer", Value: "B"},
				{Key: "Footer", Value: "D"},
				{Key: "Footer", Value: "A"},
				{Key: "TAG", Value: "fourth"},
				{Key: "TAG", Value: "third"},
				{Key: "TAG", Value: "second"},
				{Key: "TAG", Value: "first"},
			})
		})
	})
}
