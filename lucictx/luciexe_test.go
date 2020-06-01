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

package lucictx

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLUCIExe(t *testing.T) {
	Convey(`test luciexe`, t, func() {
		ctx := context.Background()

		Convey(`can get from empty ctx`, func() {
			So(GetLUCIExe(ctx), ShouldResembleProto, (*LUCIExe)(nil))
		})

		Convey(`can set in ctx`, func() {
			ctx = SetLUCIExe(ctx, &LUCIExe{CacheDir: "hello"})
			So(GetLUCIExe(ctx), ShouldResembleProto, &LUCIExe{CacheDir: "hello"})

			Convey(`setting nil clears it out`, func() {
				ctx = SetLUCIExe(ctx, nil)
				So(GetLUCIExe(ctx), ShouldResembleProto, (*LUCIExe)(nil))
			})
		})
	})
}
