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
	"strings"
	"testing"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit/metadata"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExtractOptions(t *testing.T) {
	t.Parallel()

	Convey("ExtractOptions works", t, func() {
		extract := func(msg string) *Options {
			lines := strings.Split(strings.TrimSpace(msg), "\n")
			for i, l := range lines {
				lines[i] = strings.TrimSpace(l)
			}
			msg = strings.Join(lines, "\n")
			return ExtractOptions(&changelist.Snapshot{Metadata: metadata.Extract(msg)})
		}
		Convey("Default", func() {
			So(extract(`
				CL title.

				SOME_COMMIT_TAG=ignored-by-CV
				BUG=1
				YES_BUG=is also a tag

				Gerrit-Or-Git-Style-Footer: but its key not recognized by CV either.
				Change-Id: Ideadbeef
				Bug: 1
				Yes-Bug-Above: is a Git/Gerrit footer
      `), ShouldResembleProto, &Options{})
		})

		Convey("No-Tree-Checks", func() {
			So(extract(`
				CL title.

				No-Tree-Checks: true
      `), ShouldResembleProto, &Options{
				SkipTreeChecks: true,
			})
			So(extract(`
				CL title.

				NOTREECHECKS=true
      `), ShouldResembleProto, &Options{
				SkipTreeChecks: true,
			})
		})

		Convey("No-Try / No-Presubmit", func() {
			So(extract(`
				CL title.

				NOPRESUBMIT=true

				No-Try: true
      `), ShouldResembleProto, &Options{
				SkipTryjobs:   true,
				SkipPresubmit: true,
			})
			So(extract(`
				CL title.

				NOTRY=true

				No-Presubmit: true
      `), ShouldResembleProto, &Options{
				SkipTryjobs:   true,
				SkipPresubmit: true,
			})
		})

		Convey("Cq-Do-Not-Cancel-Tryjobs", func() {
			So(extract(`
				CL title.

				Cq-Do-Not-Cancel-Tryjobs: true
      `), ShouldResembleProto, &Options{
				AvoidCancellingTryjobs: true,
			})
		})

		Convey("No-Equivalent-Builders", func() {
			So(extract(`
				CL title.

				No-Equivalent-Builders: true
      `), ShouldResembleProto, &Options{
				SkipEquivalentBuilders: true,
			})
		})

		Convey("If keys are repeated, any true value means true", func() {
			So(extract(`
				CL title.

				NOTRY=true
				No-Try: false
      `), ShouldResembleProto, &Options{
				SkipTryjobs: true,
			})
			So(extract(`
				CL title.

				NOTRY=false
				No-Try: true
      `), ShouldResembleProto, &Options{
				SkipTryjobs: true,
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
