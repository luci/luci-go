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

package gobmap

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGobMap(t *testing.T) {
	t.Parallel()

	Convey("UpdateProjectConfig", t, func() {
		ctx := context.Background()

		Convey("not implemented", func() {
			So(
				UpdateProjectConfig(ctx, "project-foo"),
				ShouldErrLike, "not implemented")
		})
	})

	Convey("Lookup", t, func() {
		ctx := context.Background()

		Convey("not implemented", func() {
			ids, err := Lookup(
				ctx, "foo-review.googlesource.com", "re/po", "main")
			So(ids, ShouldBeNil)
			So(err, ShouldErrLike, "not implemented")
		})
	})
}
