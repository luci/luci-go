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

package prjcfg

import (
	"testing"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGerritHost(t *testing.T) {
	t.Parallel()

	Convey("GerritHost", t, func() {
		So(GerritHost(&cfgpb.ConfigGroup_Gerrit{Url: "https://o.k"}), ShouldEqual, "o.k")
		So(GerritHost(&cfgpb.ConfigGroup_Gerrit{Url: "https://strip.sla.shes/"}), ShouldEqual, "strip.sla.shes")

		So(func() { GerritHost(&cfgpb.ConfigGroup_Gerrit{}) }, ShouldPanic)
		So(func() { GerritHost(&cfgpb.ConfigGroup_Gerrit{Url: "no.scheme"}) }, ShouldPanic)
	})
}
