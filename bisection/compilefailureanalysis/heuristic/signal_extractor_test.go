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

package heuristic

import (
	"context"
	"testing"

	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestExtractSignal(t *testing.T) {
	t.Parallel()
	c := context.Background()
	ftt.Run("Extract Ninja Log", t, func(t *ftt.Test) {
		t.Run("No Log Should throw error", func(t *ftt.Test) {
			failureLog := &model.CompileLogs{}
			_, e := ExtractSignals(c, failureLog)
			assert.Loosely(t, e, should.NotBeNil)
		})
		t.Run("No output", func(t *ftt.Test) {
			failureLog := &model.CompileLogs{
				NinjaLog: &model.NinjaLog{
					Failures: []*model.NinjaLogFailure{
						{
							Rule:         "CXX",
							OutputNodes:  []string{"n1", "n2"},
							Dependencies: []string{"a/b/../c.d", "../../a.b"},
						},
						{
							Rule:         "CC",
							OutputNodes:  []string{"n3", "n4"},
							Dependencies: []string{"x/y/./z", "d\\e\\f"},
						},
					},
				},
			}
			signal, e := ExtractSignals(c, failureLog)
			assert.Loosely(t, e, should.BeNil)
			assert.Loosely(t, signal, should.Resemble(&model.CompileFailureSignal{
				Nodes: []string{"n1", "n2", "n3", "n4"},
				Edges: []*model.CompileFailureEdge{
					{
						Rule:         "CXX",
						OutputNodes:  []string{"n1", "n2"},
						Dependencies: []string{"a/c.d", "a.b"},
					},
					{
						Rule:         "CC",
						OutputNodes:  []string{"n3", "n4"},
						Dependencies: []string{"x/y/z", "d/e/f"},
					},
				},
			}))
			t.Run("Python patterns", func(t *ftt.Test) {
				failureLog := &model.CompileLogs{
					NinjaLog: &model.NinjaLog{
						Failures: []*model.NinjaLogFailure{
							{
								Rule:         "CXX",
								OutputNodes:  []string{"n1", "n2"},
								Dependencies: []string{"a/b/../c.d", "../../a.b"},
								Output: `
method1 at path/a.py:1
message1
method2 at ../path/b.py:2
message2
method3 at path/a.py:3
message3
blablaError: blabla...

blabla

File "path/a.py", line 14, in method1
message1
File "path/c.py", line 34, in method2
message2
blabalError: blabla...`,
							},
						},
					},
				}
				signal, e := ExtractSignals(c, failureLog)
				assert.Loosely(t, e, should.BeNil)
				assert.Loosely(t, signal, should.Resemble(&model.CompileFailureSignal{
					Nodes: []string{"n1", "n2"},
					Edges: []*model.CompileFailureEdge{
						{
							Rule:         "CXX",
							OutputNodes:  []string{"n1", "n2"},
							Dependencies: []string{"a/c.d", "a.b"},
						},
					},
					Files: map[string][]int{
						"path/a.py": {1, 3, 14},
						"path/b.py": {2},
						"path/c.py": {34},
					},
				}))
			})

			t.Run("NonPython patterns", func(t *ftt.Test) {
				failureLog := &model.CompileLogs{
					NinjaLog: &model.NinjaLog{
						Failures: []*model.NinjaLogFailure{
							{
								Rule:         "CXX",
								OutputNodes:  []string{"obj/a/b/test.c.o"},
								Dependencies: []string{"../../a/b/c.c", "../../a.b"},
								Output: `/b/build/goma/gomacc blabla ... -c ../../a/b/c.c -o obj/a/b/test.c.o
../../a/b/c.c:307:44: error:no member 'kEnableExtensionInfoDialog' ...
Error
../../d.cpp error:Undeclared variable ...
Error
blah blah c:\\a\\b.txt:12 error
Error c:\a\b.txt(123) blah blah
D:\\x\\y.cc[line 456]
//BUILD.gn
1 error generated.`,
							},
						},
					},
				}
				signal, e := ExtractSignals(c, failureLog)
				assert.Loosely(t, e, should.BeNil)
				assert.Loosely(t, signal, should.Resemble(&model.CompileFailureSignal{
					Nodes: []string{"obj/a/b/test.c.o"},
					Edges: []*model.CompileFailureEdge{
						{
							Rule:         "CXX",
							OutputNodes:  []string{"obj/a/b/test.c.o"},
							Dependencies: []string{"a/b/c.c", "a.b"},
						},
					},
					Files: map[string][]int{
						"a/b/c.c":    {307},
						"d.cpp":      {},
						"c:/a/b.txt": {12, 123},
						"D:/x/y.cc":  {456},
						"BUILD.gn":   {},
					},
				}))
			})
		})
	})

	ftt.Run("Extract Stdout Log", t, func(t *ftt.Test) {
		t.Run("Stdout", func(t *ftt.Test) {
			failureLog := &model.CompileLogs{
				NinjaLog: nil,
				StdOutLog: `
[1832/2467 | 117.498] CXX obj/a/b/test.file.o
blabla...
FAILED: obj/a/b/test.c.o
/b/build/goma/gomacc blabla ... -c ../../a/b/c.cc -o obj/a/b/test.c.o
../../a/b/c.cc:307:44: error: no member 'kEnableExtensionInfoDialog' ...
1 error generated.
x/y/not_in_signal.cc
FAILED: obj/a/b/test.d.o
/b/build/goma/gomacc blabla ... -c ../../a/b/d.cc -o obj/a/b/test.d.o
../../a/b/d.cc:123:44: error: no member 'kEnableExtensionInfoDialog' ...
blabla...
1 error generated.
FAILED: obj/a/b/test.e.o "another node 1" obj/a/b/test.f.o "another node 2" "another node 3"
/b/build/goma/gomacc ... ../../a/b/e.cc ... obj/a/b/test.e.o
../../a/b/e.cc:79:44: error: no member 'kEnableExtensionInfoDialog' ...
blabla...
ninja: build stopped: subcommand failed.

/b/build/goma/goma_ctl.sh stat
blabla...
				`,
			}
			signal, e := ExtractSignals(c, failureLog)
			assert.Loosely(t, e, should.BeNil)
			assert.Loosely(t, signal, should.Resemble(&model.CompileFailureSignal{
				Files: map[string][]int{
					"a/b/c.cc":         {307},
					"a/b/d.cc":         {123},
					"a/b/e.cc":         {79},
					"obj/a/b/test.c.o": {},
					"obj/a/b/test.d.o": {},
					"obj/a/b/test.e.o": {},
				},
				Nodes: []string{
					"obj/a/b/test.c.o",
					"obj/a/b/test.d.o",
					"obj/a/b/test.e.o",
					"another node 1",
					"obj/a/b/test.f.o",
					"another node 2",
					"another node 3",
				},
			}))
		})
	})
}
