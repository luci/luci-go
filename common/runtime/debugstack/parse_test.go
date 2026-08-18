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
	"path"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/runtime/debugstack"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseRoundTrip(t *testing.T) {
	t.Parallel()

	inputs, err := filepath.Glob(filepath.Join("testdata", "test-fixtures", "*.txt"))
	assert.NoErr(t, err)

	for _, fname := range inputs {
		t.Run(fname, func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(fname)
			assert.NoErr(t, err)
			dataS := string(data)

			parsed := debugstack.Parse(data)
			check.That(t, parsed.String(), should.Equal(dataS))

			parsed = debugstack.ParseString(dataS)
			check.That(t, parsed.String(), should.Equal(dataS))
		})
	}
}

func TestParseStatistics(t *testing.T) {
	run := func(expected map[debugstack.FrameKind]int) func(t *testing.T) {
		return func(t *testing.T) {
			t.Parallel()

			fname := filepath.Join("testdata", "test-fixtures", path.Base(t.Name()))
			data, err := os.ReadFile(fname)
			assert.NoErr(t, err)

			parsed := debugstack.Parse(data)
			if !check.That(t, parsed.Stats(), should.Match(expected)) {
				for _, frame := range parsed {
					t.Logf("%#v", frame)
				}
			}
		}
	}

	t.Run("ancestors.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind:   1,
		debugstack.GoroutineAncestryKind: 6,
		debugstack.CreatedByFrameKind:    6,
		debugstack.StackFrameKind:        42,
	}))

	t.Run("cgo.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.CreatedByFrameKind:  1,
		debugstack.StackFrameKind:      5,
	}))

	t.Run("empty_input.txt", run(nil))

	t.Run("empty_stack.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
	}))

	t.Run("errorhandling.txt", run(map[debugstack.FrameKind]int{
		debugstack.CreatedByFrameKind:  1,
		debugstack.GoroutineHeaderKind: 1,
		debugstack.StackFrameKind:      1,
	}))

	t.Run("frameselided.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.FramesElidedKind:    1,
		debugstack.StackFrameKind:      100,
	}))

	t.Run("frameselided_goroutine.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.FramesElidedKind:    1,
		debugstack.CreatedByFrameKind:  1,
		debugstack.StackFrameKind:      100,
	}))

	t.Run("go121_createdby.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.StackFrameKind:      2,
		debugstack.CreatedByFrameKind:  1,
	}))

	t.Run("lockedm.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.StackFrameKind:      5,
		debugstack.CreatedByFrameKind:  1,
	}))

	t.Run("partial_createdby.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.StackFrameKind:      1,
		debugstack.CreatedByFrameKind:  1,
	}))

	t.Run("partial_stack.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.StackFrameKind:      1,
	}))

	t.Run("waitsince.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.CreatedByFrameKind:  1,
		debugstack.StackFrameKind:      15,
	}))

	t.Run("windows.txt", run(map[debugstack.FrameKind]int{
		debugstack.GoroutineHeaderKind: 1,
		debugstack.StackFrameKind:      1,
	}))
}

func TestInvalidParse(t *testing.T) {
	t.Parallel()

	t.Run(`junk`, func(t *testing.T) {
		t.Parallel()

		data := []byte("afharviraear\naatr\nartaronewoHveeeee a\n tarsTarion")
		parsed := debugstack.Parse(data)
		check.That(t, parsed.String(), should.Equal(string(data)))
		check.That(t, parsed.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.UnknownFrameKind: 4,
		}))
	})

	t.Run(`no newline`, func(t *testing.T) {
		t.Parallel()

		data := []byte("afharviraearaatrartaronewoHveeeee a tarsTarion")
		parsed := debugstack.Parse(data)
		check.That(t, parsed.String(), should.Equal(string(data)))
		check.That(t, parsed.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.UnknownFrameKind: 1,
		}))
	})

	t.Run(`no fallback`, func(t *testing.T) {
		t.Parallel()

		data := []byte("[some randomline\npath/to/pkg.Func(...)\n\t/path/to/file.go:100 +0x8c")
		parsed := debugstack.Parse(data)
		check.That(t, parsed.String(), should.Equal(string(data)))
		check.That(t, parsed.Stats(), should.Match(map[debugstack.FrameKind]int{
			debugstack.UnknownFrameKind: 1,
			debugstack.StackFrameKind:   1,
		}))
	})
}
