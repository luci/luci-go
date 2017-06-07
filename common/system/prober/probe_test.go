// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prober

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/common/system/environ"
	"github.com/luci/luci-go/common/testing/testfs"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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

	realExec, err := os.Executable()
	if err != nil {
		t.Fatalf("failed to resolve the real executable: %s", err)
	}
	realExecStat, err := os.Stat(realExec)
	if err != nil {
		t.Fatalf("failed to stat the real executable %q: %s", realExec, err)
	}

	Convey(`With a temporary directory`, t, testfs.MustWithTempDir(t, "resolve_self", func(tdir string) {
		// Set up a base probe.
		probe := Probe{
			Target: "git",
		}

		Convey(`Will resolve our executable.`, func() {
			So(probe.ResolveSelf(""), ShouldBeNil)
			So(probe.Self, ShouldEqual, realExec)
			So(os.SameFile(probe.SelfStat, realExecStat), ShouldBeTrue)
		})

		Convey(`When "self" is a symlink to the executable`, func() {
			symSelf := filepath.Join(tdir, "me")
			if err := os.Symlink(realExec, symSelf); err != nil {
				t.Skipf("Could not create symlink %q => %q: %s", realExec, symSelf, err)
				return
			}

			So(probe.ResolveSelf(symSelf), ShouldBeNil)
			So(probe.Self, ShouldEqual, symSelf)
			So(os.SameFile(probe.SelfStat, realExecStat), ShouldBeTrue)
		})
	}))
}

// TestFindSystemGit tests the ability to locate the "git" command in PATH.
func TestSystemProbe(t *testing.T) {
	t.Parallel()

	var envBase environ.Env
	var selfEXESuffix, otherEXESuffix string
	if runtime.GOOS == "windows" {
		selfEXESuffix = ".exe"
		otherEXESuffix = ".bat"
		envBase.Set("PATHEXT", strings.Join([]string{".com", ".exe", ".bat", ".ohai"}, string(os.PathListSeparator)))
	}

	Convey(`With a fake PATH setup`, t, testfs.MustWithTempDir(t, "system_probe", func(tdir string) {
		c := baseTestContext()

		createExecutable := func(relPath string) (dir string, path string) {
			path = filepath.Join(tdir, filepath.FromSlash(relPath))
			dir = filepath.Dir(path)
			if err := os.MkdirAll(dir, 0755); err != nil {
				t.Fatalf("Failed to create base directory [%s]: %s", dir, err)
			}
			if err := ioutil.WriteFile(path, []byte("fake"), 0755); err != nil {
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

			CheckWrapper: func(c context.Context, path string, env environ.Env) (bool, error) {
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

		Convey(`Can identify the next Git when it follows self in PATH.`, func() {
			setPATH(selfDir, selfDir, fooDir, wrapperDir, otherDir, nonexistDir)

			git, err := probe.Locate(c, "", env)
			So(err, ShouldBeNil)
			So(git, shouldBeSameFileAs, fooGit)
			So(wrapperChecks, ShouldEqual, 1)
		})

		Convey(`When Git precedes self in PATH`, func() {
			setPATH(fooDir, selfDir, wrapperDir, otherDir, nonexistDir)

			Convey(`Will identify Git`, func() {
				git, err := probe.Locate(c, "", env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, fooGit)
				So(wrapperChecks, ShouldEqual, 1)
			})

			Convey(`Will prefer an override Git`, func() {
				probe.RelativePathOverride = []string{
					"override/reldir", // (see "overrideGit")
				}

				git, err := probe.Locate(c, "", env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, overrideGit)
				So(wrapperChecks, ShouldEqual, 1)
			})
		})

		Convey(`Can identify the next Git when self does not exist.`, func() {
			setPATH(wrapperDir, selfDir, wrapperDir, otherDir, nonexistDir, fooDir)

			probe.Self = nonexistGit
			git, err := probe.Locate(c, "", env)
			So(err, ShouldBeNil)
			So(git, shouldBeSameFileAs, fooGit)
			So(wrapperChecks, ShouldEqual, 2)
		})

		Convey(`With PATH setup pointing to a wrapper, self, and then the system Git`, func() {
			// NOTE: wrapperDir is repeated, but it will only count towards one check,
			// since we cache checks on a per-directory basis.
			setPATH(wrapperDir, wrapperDir, selfDir, otherDir, fooDir, nonexistDir)

			Convey(`Will prefer the cached value.`, func() {
				git, err := probe.Locate(c, fooGit, env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, fooGit)
				So(wrapperChecks, ShouldEqual, 0)
			})

			Convey(`Will ignore the cached value if it is self.`, func() {
				git, err := probe.Locate(c, selfGit, env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, fooGit)
				So(wrapperChecks, ShouldEqual, 2)
			})

			Convey(`Will ignore the cached value if it does not exist.`, func() {
				git, err := probe.Locate(c, nonexistGit, env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, fooGit)
				So(wrapperChecks, ShouldEqual, 2)
			})

			Convey(`Will skip the wrapper and identify the system Git.`, func() {
				git, err := probe.Locate(c, "", env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, fooGit)
				So(wrapperChecks, ShouldEqual, 2)
			})
		})

		Convey(`Will skip everything if the wrapper check fails.`, func() {
			setPATH(wrapperDir, brokenDir, selfDir, otherDir, fooDir, nonexistDir)

			git, err := probe.Locate(c, "", env)
			So(err, ShouldBeNil)
			So(git, shouldBeSameFileAs, fooGit)
			So(wrapperChecks, ShouldEqual, 3)
		})

		Convey(`Will fail if cannot find another Git in PATH.`, func() {
			setPATH(selfDir, otherDir, nonexistDir)

			_, err := probe.Locate(c, "", env)
			So(err, ShouldErrLike, "could not find target in system")
			So(wrapperChecks, ShouldEqual, 0)
		})

		Convey(`When a symlink is created`, func() {
			conveyFn := Convey
			if err := os.Symlink(selfGit, filepath.Join(otherDir, filepath.Base(selfGit))); err != nil {
				t.Logf("Failed to create symlink; skipping symlink test: %s", err)
				conveyFn = SkipConvey
			}

			conveyFn(`Will ignore symlink because it's the same file.`, func() {
				setPATH(selfDir, otherDir, fooDir, wrapperDir)
				git, err := probe.Locate(c, "", env)
				So(err, ShouldBeNil)
				So(git, shouldBeSameFileAs, fooGit)
				So(wrapperChecks, ShouldEqual, 1)
			})
		})
	}))
}

func shouldBeSameFileAs(actual interface{}, expected ...interface{}) string {
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
