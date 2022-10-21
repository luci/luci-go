// Copyright 2022 The LUCI Authors.
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

package listener

import (
	"testing"

	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	listenerpb "go.chromium.org/luci/cv/settings/listener"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIsPubsubEnabled(t *testing.T) {
	t.Parallel()

	Convey("IsPubsubEnabled", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		settings := &listenerpb.Settings{
			EnabledProjectRegexps: []string{"fo?", "bar.*", "deed"},
		}
		So(srvcfg.SetTestListenerConfig(ctx, settings), ShouldBeNil)
		check := func(prj string) bool {
			yes, err := IsPubsubEnabled(ctx, prj)
			So(err, ShouldBeNil)
			return yes
		}

		Convey("Returns true", func() {
			So(check("fo"), ShouldBeTrue)
			So(check("bar123"), ShouldBeTrue)
			So(check("deed"), ShouldBeTrue)
		})

		Convey("Returns false", func() {
			So(check("d"), ShouldBeFalse)
			So(check("ba"), ShouldBeFalse)
		})

		Convey("Performs full matches", func() {
			So(check("ddeed"), ShouldBeFalse)
		})
	})
}
