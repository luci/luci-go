// Copyright 2023 The LUCI Authors.
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

package run

import (
	"testing"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"

	. "github.com/smartystreets/goconvey/convey"
)

func TestShouldSubmit(t *testing.T) {
	t.Parallel()

	Convey("ShouldSubmit works", t, func() {
		Convey("Returns true for full run", func() {
			r := &Run{Mode: FullRun}
			So(ShouldSubmit(r), ShouldBeTrue)
		})
		Convey("Returns false for dry run", func() {
			r := &Run{Mode: DryRun}
			So(ShouldSubmit(r), ShouldBeFalse)
		})
		Convey("Returns true for user defined mode with +2 cq vote", func() {
			r := &Run{
				Mode: "CUSTOM_RUN",
				ModeDefinition: &cfgpb.Mode{
					Name:            "CUSTOM_RUN",
					CqLabelValue:    2,
					TriggeringLabel: "CUSTOM",
					TriggeringValue: 1,
				}}
			So(ShouldSubmit(r), ShouldBeTrue)
		})
		Convey("Returns false for user defined mode without +2 cq vote", func() {
			r := &Run{
				Mode: "CUSTOM_RUN",
				ModeDefinition: &cfgpb.Mode{
					Name:            "CUSTOM_RUN",
					CqLabelValue:    1,
					TriggeringLabel: "CUSTOM",
					TriggeringValue: 1,
				}}
			So(ShouldSubmit(r), ShouldBeFalse)
		})
	})
}
