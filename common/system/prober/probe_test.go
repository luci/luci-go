// Copyright 2017 The LUCI Authors.
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

package prober

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/system/environ"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func baseTestContext() context.Context {
	c := context.Background()
	c = gologger.StdConfig.Use(c)
	c = logging.SetLevel(c, logging.Debug)
	return c
}

// TestFindSystemGit tests the ability to resolve the current executable.
func TestResolveSelf(t *testing.T) {
	t.Parallel()

	// On OSX temp dir is a symlink and it interferes with the test.
	evalSymlinks := func(p string) string {
		p, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatalf("eval symlinks: %s", err)
		}
		return p
	}

	realExec, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to resolve the real executable: %s", err)
	}
	realExec = evalSymlinks(realExec)

	realExecStat, err := os.Stat(realExec)
	if err != nil {
		t.Fatalf("failed to stat the real executable %q: %s", realExec, err)
	}

	ftt.Run(`With a temporary directory`, t, func(t *ftt.Test) {
		tdir := evalSymlinks(t.TempDir())

		// Set up a base probe.
		probe := Probe{
			Target: "git",
		}

		t.Run(`Will resolve our executable.`, func(t *ftt.Test) {
			assert.Loosely(t, probe.ResolveSelf(), should.BeNil)
			assert.Loosely(t, probe.Self, should.Equal(realExec))
			assert.Loosely(t, os.SameFile(probe.SelfStat, realExecStat), should.BeTrue)
		})

		t.Run(`When "self" is a symlink to the executable`, func(t *ftt.Test) {
			symSelf := filepath.Join(tdir, "me")
			if err := os.Symlink(realExec, symSelf); err != nil {
				t.Skipf("Could not create symlink %q => %q: %s", realExec, symSelf, err)
				return
			}

			assert.Loosely(t, probe.ResolveSelf(), should.BeNil)
			assert.Loosely(t, probe.Self, should.Equal(realExec))
			assert.Loosely(t, os.SameFile(probe.SelfStat, realExecStat), should.BeTrue)
		})
	})
}

// TestFindSystemGit tests the ability to locate the "git" command in PATH.
func TestSystemProbe(t *testing.T) {
	t.Parallel()

	envBase := environ.New(nil)
	var selfEXESuffix, otherEXESuffix string
	if runtime.GOOS == "windows" {
		selfEXESuffix = ".exe"
		otherEXESuffix = ".bat"
		envBase.Set("PATHEXT", strings.Join([]string{".com", ".exe", ".bat", ".ohai"}, string(os.PathListSeparator)))
	}

	ftt.Run(`With a fake PATH setup`, t, func(t *ftt.Test) {
		tdir := t.TempDir()
		c := baseTestContext()

		createExecutable := func(relPath string) (dir string, path string) {
			path = filepath.Join(tdir, filepath.FromSlash(relPath))
			dir = filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create base directory [%s]: %s", dir, err)
			}
			if err := os.WriteFile(path, []byte("fake"), 0755); err != nil {
				t.Fatalf("Failed to create executable: %s", err)
			}
			return
		}

		// Construct a fake filesystem rooted in "tdir".
		var (
			selfDir, selfGit = createExecutable("self/git" + selfEXESuffix)
			_, overrideGit   = createExecutable("self/override/reldir/git" + otherEXESuffix)
			fooDir, fooGit   = createExecutable("foo/git" + otherEXESuffix)
			wrapperDir, _    = createExecutable("wrapper/git" + otherEXESuffix)
			brokenDir, _     = createExecutable("broken/git" + otherEXESuffix)
			otherDir, _      = createExecutable("other/not_git")
			nonexistDir      = filepath.Join(tdir, "nonexist")
			nonexistGit      = filepath.Join(nonexistDir, "git"+otherEXESuffix)
		)

		// Set up a base probe.
		wrapperChecks := 0
		probe := Probe{
			Target: "git",

			CheckWrapper: func(ctx context.Context, path string, env environ.Env) (bool, error) {
				wrapperChecks++
				switch filepath.Dir(path) {
				case wrapperDir, selfDir:
					return true, nil
				case brokenDir:
					return false, errors.New("broken")
				default:
					return false, nil
				}
			},
		}

		selfGitStat, err := os.Stat(selfGit)
		if err != nil {
			t.Fatalf("failed to stat self Git %q: %s", selfGit, err)
		}
		probe.Self, probe.SelfStat = selfGit, selfGitStat

		env := envBase.Clone()
		setPATH := func(v ...string) {
			env.Set("PATH", strings.Join(v, string(os.PathListSeparator)))
		}

		t.Run(`Can identify the next Git when it follows self in PATH.`, func(t *ftt.Test) {
			setPATH(selfDir, selfDir, fooDir, wrapperDir, otherDir, nonexistDir)

			git, err := probe.Locate(c, "", env)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
			assert.Loosely(t, wrapperChecks, should.Equal(1))
		})

		t.Run(`When Git precedes self in PATH`, func(t *ftt.Test) {
			setPATH(fooDir, selfDir, wrapperDir, otherDir, nonexistDir)

			t.Run(`Will identify Git`, func(t *ftt.Test) {
				git, err := probe.Locate(c, "", env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
				assert.Loosely(t, wrapperChecks, should.Equal(1))
			})

			t.Run(`Will prefer an override Git`, func(t *ftt.Test) {
				probe.RelativePathOverride = []string{
					"override/reldir", // (see "overrideGit")
				}

				git, err := probe.Locate(c, "", env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(overrideGit))
				assert.Loosely(t, wrapperChecks, should.Equal(1))
			})
		})

		t.Run(`Can identify the next Git when self does not exist.`, func(t *ftt.Test) {
			setPATH(wrapperDir, selfDir, wrapperDir, otherDir, nonexistDir, fooDir)

			probe.Self = nonexistGit
			git, err := probe.Locate(c, "", env)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
			assert.Loosely(t, wrapperChecks, should.Equal(2))
		})

		t.Run(`With PATH setup pointing to a wrapper, self, and then the system Git`, func(t *ftt.Test) {
			// NOTE: wrapperDir is repeated, but it will only count towards one check,
			// since we cache checks on a per-directory basis.
			setPATH(wrapperDir, wrapperDir, selfDir, otherDir, fooDir, nonexistDir)

			t.Run(`Will prefer the cached value.`, func(t *ftt.Test) {
				git, err := probe.Locate(c, fooGit, env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
				assert.Loosely(t, wrapperChecks, should.BeZero)
			})

			t.Run(`Will ignore the cached value if it is self.`, func(t *ftt.Test) {
				git, err := probe.Locate(c, selfGit, env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
				assert.Loosely(t, wrapperChecks, should.Equal(2))
			})

			t.Run(`Will ignore the cached value if it does not exist.`, func(t *ftt.Test) {
				git, err := probe.Locate(c, nonexistGit, env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
				assert.Loosely(t, wrapperChecks, should.Equal(2))
			})

			t.Run(`Will skip the wrapper and identify the system Git.`, func(t *ftt.Test) {
				git, err := probe.Locate(c, "", env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
				assert.Loosely(t, wrapperChecks, should.Equal(2))
			})
		})

		t.Run(`Will skip everything if the wrapper check fails.`, func(t *ftt.Test) {
			setPATH(wrapperDir, brokenDir, selfDir, otherDir, fooDir, nonexistDir)

			git, err := probe.Locate(c, "", env)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
			assert.Loosely(t, wrapperChecks, should.Equal(3))
		})

		t.Run(`Will fail if cannot find another Git in PATH.`, func(t *ftt.Test) {
			setPATH(selfDir, otherDir, nonexistDir)

			_, err := probe.Locate(c, "", env)
			assert.Loosely(t, err, should.ErrLike("could not find target in system"))
			assert.Loosely(t, wrapperChecks, should.BeZero)
		})

		t.Run(`When a symlink is created`, func(t *ftt.Test) {
			t.Run(`Will ignore symlink because it's the same file.`, func(t *ftt.Test) {
				if err := os.Symlink(selfGit, filepath.Join(otherDir, filepath.Base(selfGit))); err != nil {
					t.Skipf("Failed to create symlink; skipping symlink test: %s", err)
				}

				setPATH(selfDir, otherDir, fooDir, wrapperDir)
				git, err := probe.Locate(c, "", env)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, git, convey.Adapt(shouldBeSameFileAs)(fooGit))
				assert.Loosely(t, wrapperChecks, should.Equal(1))
			})
		})
	})
}

func shouldBeSameFileAs(actual any, expected ...any) string {
	aPath, ok := actual.(string)
	if !ok {
		return "actual must be a path string"
	}

	if len(expected) != 1 {
		return "exactly one expected path string must be provided"
	}
	expPath, ok := expected[0].(string)
	if !ok {
		return "expected must be a path string"
	}

	aSt, err := os.Stat(aPath)
	if err != nil {
		return fmt.Sprintf("failed to stat actual [%s]: %s", aPath, err)
	}
	expSt, err := os.Stat(expPath)
	if err != nil {
		return fmt.Sprintf("failed to stat expected [%s]: %s", expPath, err)
	}

	if !os.SameFile(aSt, expSt) {
		return fmt.Sprintf("[%s] is not the same file as [%s]", expPath, aPath)
	}
	return ""
}
