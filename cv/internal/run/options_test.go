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

package run

import (
	"testing"

	"go.chromium.org/luci/common/data/text"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExtractOptions(t *testing.T) {
	t.Parallel()

	Convey("ExtractOptions works", t, func() {
		Convey("Default", func() {
			So(ExtractOptions(text.Doc(`
				CL title.

				SOME_COMMIT_TAG=ignored-by-CV
				BUG=1
				YES_BUG=is also a tag

				Gerrit-Or-Git-Style-Footer: but its key not recognized by CV either.
				Change-Id: Ideadbeef
				Bug: 1
				Yes-Bug-Above: is a Git/Gerrit footer
      `)), ShouldResembleProto, &Options{})
		})

		Convey("No-Tree-Checks", func() {
			So(ExtractOptions(text.Doc(`
				CL title.

				No-Tree-Checks: true
      `)), ShouldResembleProto, &Options{
				SkipTreeChecks: true,
			})
			So(ExtractOptions(text.Doc(`
				CL title.

				NOTREECHECKS=true
      `)), ShouldResembleProto, &Options{
				SkipTreeChecks: true,
			})
		})

		Convey("No-Try / No-Presubmit", func() {
			So(ExtractOptions(text.Doc(`
				CL title.

				NOPRESUBMIT=true

				No-Try: true
      `)), ShouldResembleProto, &Options{
				SkipTryjobs:   true,
				SkipPresubmit: true,
			})
			So(ExtractOptions(text.Doc(`
				CL title.

				NOTRY=true

				No-Presubmit: true
      `)), ShouldResembleProto, &Options{
				SkipTryjobs:   true,
				SkipPresubmit: true,
			})
		})

		Convey("Cq-Do-Not-Cancel-Tryjobs", func() {
			So(ExtractOptions(text.Doc(`
				CL title.

				Cq-Do-Not-Cancel-Tryjobs: true
      `)), ShouldResembleProto, &Options{
				AvoidCancellingTryjobs: true,
			})
		})

		Convey("No-Equivalent-Builders", func() {
			So(ExtractOptions(text.Doc(`
				CL title.

				No-Equivalent-Builders: true
      `)), ShouldResembleProto, &Options{
				SkipEquivalentBuilders: true,
			})
		})
	})
}

func TestMergeOptions(t *testing.T) {
	t.Parallel()

	Convey("MergeOptions works", t, func() {
		o := &Options{}
		So(MergeOptions(o, nil), ShouldResembleProto, o)

		a := &Options{
			SkipTreeChecks:         true,
			AvoidCancellingTryjobs: true,
		}
		So(MergeOptions(a, o), ShouldResembleProto, a)

		b := &Options{
			SkipTreeChecks:         true,
			SkipEquivalentBuilders: true,
		}
		So(MergeOptions(a, b), ShouldResembleProto, &Options{
			SkipTreeChecks:         true,
			SkipEquivalentBuilders: true,
			AvoidCancellingTryjobs: true,
		})
	})
}
