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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit/metadata"
)

func TestExtractOptions(t *testing.T) {
	t.Parallel()

	ftt.Run("ExtractOptions works", t, func(t *ftt.Test) {
		extract := func(msg string) *Options {
			lines := strings.Split(strings.TrimSpace(msg), "\n")
			for i, l := range lines {
				lines[i] = strings.TrimSpace(l)
			}
			msg = strings.Join(lines, "\n")
			return ExtractOptions(&changelist.Snapshot{Metadata: metadata.Extract(msg)})
		}
		t.Run("Default", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				SOME_COMMIT_TAG=ignored-by-CV
				BUG=1
				YES_BUG=is also a tag

				Gerrit-Or-Git-Style-Footer: but its key not recognized by CV either.
				Change-Id: Ideadbeef
				Bug: 1
				Yes-Bug-Above: is a Git/Gerrit footer
      `), should.Match(&Options{}))
		})

		t.Run("No-Tree-Checks", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				No-Tree-Checks: true
      `), should.Match(&Options{
				SkipTreeChecks: true,
			}))
			assert.Loosely(t, extract(`
				CL title.

				NOTREECHECKS=true
      `), should.Match(&Options{
				SkipTreeChecks: true,
			}))
		})

		t.Run("No-Try / No-Presubmit", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				NOPRESUBMIT=true

				No-Try: true
      `), should.Match(&Options{
				SkipTryjobs:   true,
				SkipPresubmit: true,
			}))
			assert.Loosely(t, extract(`
				CL title.

				NOTRY=true

				No-Presubmit: true
      `), should.Match(&Options{
				SkipTryjobs:   true,
				SkipPresubmit: true,
			}))
		})

		t.Run("Cq-Do-Not-Cancel-Tryjobs", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				Cq-Do-Not-Cancel-Tryjobs: true
      `), should.Match(&Options{
				AvoidCancellingTryjobs: true,
			}))
		})

		t.Run("No-Equivalent-Builders", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				No-Equivalent-Builders: true
      `), should.Match(&Options{
				SkipEquivalentBuilders: true,
			}))
		})

		t.Run("Cq-Include-Trybots/CQ_INCLUDE_TRYBOTS", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				Cq-Include-Trybots: project/bucket:builder1,builder2;project2/bucket:builder3
				CQ_INCLUDE_TRYBOTS=project/bucket:builder4
			`), should.Match(
				&Options{
					IncludedTryjobs: []string{
						"project/bucket:builder1,builder2;project2/bucket:builder3",
						"project/bucket:builder4",
					},
				}))
		})

		t.Run("Override-Tryjobs-For-Automation", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				Override-Tryjobs-For-Automation: project/bucket:builder1,builder2;project2/bucket:builder3
				Override-Tryjobs-For-Automation: project/bucket:builder4
			`), should.Match(
				&Options{
					OverriddenTryjobs: []string{
						"project/bucket:builder4",
						"project/bucket:builder1,builder2;project2/bucket:builder3",
					},
				}))
		})

		t.Run("Cq-Cl-Tag", func(t *ftt.Test) {
			// legacy format (i.e. CQ_CL_TAG=XXX) is not supported.
			assert.Loosely(t, extract(`
				CL title.

				Cq-Cl-Tag: foo:bar
				Cq-Cl-Tag: foo:baz
				CQ_CL_TAG=another_foo:another_bar
			`), should.Match(
				&Options{
					CustomTryjobTags: []string{
						"foo:baz",
						"foo:bar",
					},
				}))
		})

		t.Run("If keys are repeated, any true value means true", func(t *ftt.Test) {
			assert.Loosely(t, extract(`
				CL title.

				NOTRY=true
				No-Try: false
      `), should.Match(&Options{
				SkipTryjobs: true,
			}))
			assert.Loosely(t, extract(`
				CL title.

				NOTRY=false
				No-Try: true
      `), should.Match(&Options{
				SkipTryjobs: true,
			}))
		})

	})
}

func TestMergeOptions(t *testing.T) {
	t.Parallel()

	ftt.Run("MergeOptions works", t, func(t *ftt.Test) {
		o := &Options{}
		assert.That(t, MergeOptions(o, nil), should.Match(o))

		a := &Options{
			SkipTreeChecks:         true,
			AvoidCancellingTryjobs: true,
		}
		assert.That(t, MergeOptions(a, o), should.Match(a))

		b := &Options{
			SkipTreeChecks:         true,
			SkipEquivalentBuilders: true,
		}
		assert.That(t, MergeOptions(a, b), should.Match(&Options{
			SkipTreeChecks:         true,
			SkipEquivalentBuilders: true,
			AvoidCancellingTryjobs: true,
		}))
	})
}
