// Copyright 2025 The LUCI Authors.
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

package debugstack_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"go.chromium.org/luci/common/runtime/debugstack"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFilter(t *testing.T) {
	t.Parallel()

	traceData, err := os.ReadFile(filepath.Join("testdata", "test-fixtures", "frameselided_goroutine.txt"))
	assert.NoErr(t, err)
	trace := debugstack.Parse(traceData)

	t.Run(`empty`, func(t *testing.T) {
		t.Parallel()

		check.That(t, trace, should.Match(trace.Filter(debugstack.CompiledRule{}, false)))
	})

	t.Run(`drop frames`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(debugstack.Rule{ApplyTo: debugstack.StackFrameKind, Drop: true})

		filtered := trace.Filter(rules, true)

		check.That(t, filtered.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.GoroutineHeaderKind: 1,
			debugstack.FramesElidedKind:    2,
			debugstack.CreatedByFrameKind:  1,
		}))

		check.That(t, filtered[1].Lines, should.Match([]string{
			"...100 frames elided...\n",
		}))
	})

	t.Run(`drop pkgname (stackframes)`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind,
			DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`main`)},
		})

		filtered := trace.Filter(rules, true)

		check.That(t, filtered.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.GoroutineHeaderKind: 1,
			debugstack.StackFrameKind:      3,
			debugstack.FramesElidedKind:    2,
			debugstack.CreatedByFrameKind:  1,
		}))
	})

	t.Run(`drop pkgname (stackframes+createdbyframes)`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind | debugstack.CreatedByFrameKind,
			DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`main`)},
		})

		filtered := trace.Filter(rules, true)

		check.That(t, filtered.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.GoroutineHeaderKind: 1,
			debugstack.StackFrameKind:      3,
			debugstack.FramesElidedKind:    3,
		}))
	})

	t.Run(`drop multiple pkgname`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind,
			DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`main`)},
		}, debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind,
			DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`runtime/.*`)},
		})

		filtered := trace.Filter(rules, true)

		check.That(t, filtered.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.GoroutineHeaderKind: 1,
			debugstack.FramesElidedKind:    2,
			debugstack.CreatedByFrameKind:  1,
		}))
	})

	t.Run(`dropN`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind,
			DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`main`)},
		}, debugstack.Rule{
			ApplyTo:     debugstack.StackFrameKind,
			DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`runtime/.*`)},
		}, debugstack.Rule{
			ApplyTo: debugstack.GoroutineHeaderKind,
			DropN:   100,
		})

		filtered := trace.Filter(rules, true)

		check.That(t, filtered.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.FramesElidedKind:   2,
			debugstack.CreatedByFrameKind: 1,
		}))
	})

	t.Run(`drop all`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(
			debugstack.Rule{
				ApplyTo: debugstack.FramesElidedKind | debugstack.CreatedByFrameKind | debugstack.GoroutineHeaderKind | debugstack.StackFrameKind,
				Drop:    true,
			},
			debugstack.Rule{
				ApplyTo:     debugstack.StackFrameKind,
				DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`main`)},
			}, debugstack.Rule{
				ApplyTo:     debugstack.StackFrameKind,
				DropIfInPkg: []*regexp.Regexp{regexp.MustCompile(`runtime/.*`)},
			}, debugstack.Rule{
				ApplyTo: debugstack.GoroutineHeaderKind,
				DropN:   100,
			})

		filtered := trace.Filter(rules, true)

		check.That(t, filtered.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.FramesElidedKind: 1,
		}))
	})

	t.Run(`ignore frames without pkgname`, func(t *testing.T) {
		t.Parallel()

		rules := debugstack.CompileRules(debugstack.Rule{ApplyTo: debugstack.GoroutineHeaderKind, DropIfInPkg: []*regexp.Regexp{
			regexp.MustCompile("does not matter"),
		}})
		check.That(t, trace, should.Match(trace.Filter(rules, true)))
	})
}
