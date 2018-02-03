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

package venv

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/python"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/system/environ"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/testfs"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var verbose = flag.Bool("test.gologger", false, "Enable Go logging.")

func testContext() context.Context {
	c := context.Background()
	if *verbose {
		c = gologger.StdConfig.Use(c)
		c = logging.SetLevel(c, logging.Debug)
	}
	return c
}

type resolvedInterpreter struct {
	py      *python.Interpreter
	version python.Version
}

func resolveFromPath(vers python.Version) *resolvedInterpreter {
	c := testContext()
	py, err := python.Find(c, vers, nil)
	if err != nil {
		return nil
	}
	if err := filesystem.AbsPath(&py.Python); err != nil {
		panic(err)
	}

	ri := resolvedInterpreter{
		py: py,
	}
	if ri.version, err = ri.py.GetVersion(c); err != nil {
		panic(err)
	}
	return &ri
}

var (
	pythonGeneric = resolveFromPath(python.Version{})
	python27      = resolveFromPath(python.Version{Major: 2, Minor: 7, Patch: 0})
	python3       = resolveFromPath(python.Version{Major: 3, Minor: 0, Patch: 0})
)

func TestResolvePythonInterpreter(t *testing.T) {
	t.Parallel()

	Convey(`Resolving a Python interpreter`, t, func() {
		c := testContext()
		cfg := Config{}
		s := vpython.Spec{}

		// Tests to run if we have Python 2.7 installed.
		if python27 != nil {
			Convey(`When Python 2.7 is requested, it gets resolved.`, func() {
				s.PythonVersion = "2.7"
				So(cfg.resolvePythonInterpreter(c, &s), ShouldBeNil)
				So(cfg.Python, ShouldEqual, python27.py.Python)

				vers, err := python.ParseVersion(s.PythonVersion)
				So(err, ShouldBeNil)
				So(vers.IsSatisfiedBy(python27.version), ShouldBeTrue)
			})

			Convey(`Fails when Python 9999 is requested, but a Python 2 interpreter is forced.`, func() {
				cfg.Python = python27.py.Python
				s.PythonVersion = "9999"
				So(cfg.resolvePythonInterpreter(c, &s), ShouldErrLike, "doesn't match specification")
			})
		}

		// Tests to run if we have Python 2.7 and a generic Python installed.
		if pythonGeneric != nil && python27 != nil {
			// Our generic Python resolves to a known version, so we can proceed.
			Convey(`When no Python version is specified, spec resolves to generic.`, func() {
				So(cfg.resolvePythonInterpreter(c, &s), ShouldBeNil)
				So(cfg.Python, ShouldEqual, pythonGeneric.py.Python)

				vers, err := python.ParseVersion(s.PythonVersion)
				So(err, ShouldBeNil)
				So(vers.IsSatisfiedBy(pythonGeneric.version), ShouldBeTrue)
			})
		}

		// Tests to run if we have Python 3 installed.
		if python3 != nil {
			Convey(`When Python 3 is requested, it gets resolved.`, func() {
				s.PythonVersion = "3"
				So(cfg.resolvePythonInterpreter(c, &s), ShouldBeNil)
				So(cfg.Python, ShouldEqual, python3.py.Python)

				vers, err := python.ParseVersion(s.PythonVersion)
				So(err, ShouldBeNil)
				So(vers.IsSatisfiedBy(python3.version), ShouldBeTrue)
			})

			Convey(`Fails when Python 9999 is requested, but a Python 3 interpreter is forced.`, func() {
				cfg.Python = python3.py.Python
				s.PythonVersion = "9999"
				So(cfg.resolvePythonInterpreter(c, &s), ShouldErrLike, "doesn't match specification")
			})
		}
	})
}

type setupCheckManifest struct {
	Interpreter string `json:"interpreter"`
	Pants       string `json:"pants"`
	Shirt       string `json:"shirt"`
}

func testVirtualEnvWith(t *testing.T, ri *resolvedInterpreter) {
	t.Parallel()

	if ri == nil {
		t.Skipf("No python interpreter found.")
	}

	tl, err := loadTestEnvironment(testContext(), t)
	if err != nil {
		t.Fatalf("could not set up test loader for %q: %s", ri.py.Python, err)
	}

	Convey(`Testing the VirtualEnv`, t, testfs.MustWithTempDir(t, "vpython", func(tdir string) {
		c := testContext()

		// Load the bootstrap wheels for the next part of the test.
		So(tl.ensureWheels(c, t, ri.py, tdir), ShouldBeNil)

		config := Config{
			BaseDir:    tdir,
			MaxHashLen: 4,
			SetupEnv:   environ.System(),
			Package: vpython.Spec_Package{
				Name:    "foo/bar/virtualenv",
				Version: "unresolved",
			},
			Python: ri.py.Python,
			Loader: tl,
			Spec: &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{Name: "foo/bar/shirt", Version: "unresolved"},
					{Name: "foo/bar/pants", Version: "unresolved"},
				},
			},
		}

		Convey(`Testing Setup`, func() {
			config.FailIfLocked = true
			err := With(c, config, func(c context.Context, v *Env) error {
				testScriptPath := filepath.Join(testDataDir, "setup_check.py")
				checkOut := filepath.Join(tdir, "output.json")
				cmd := v.Interpreter().IsolatedCommand(c, testScriptPath, "--json-output", checkOut)
				So(cmd.Run(), ShouldBeNil)

				var m setupCheckManifest
				So(loadJSON(checkOut, &m), ShouldBeNil)
				So(m.Interpreter, ShouldStartWith, v.Root)
				So(m.Pants, ShouldStartWith, v.Root)
				So(m.Shirt, ShouldStartWith, v.Root)
				So(v.Environment, ShouldNotBeNil)

				// We should be able to load its environment stamp.
				v.Environment = nil
				So(v.AssertCompleteAndLoad(), ShouldBeNil)
				So(v.Environment, ShouldNotBeNil)
				So(len(v.Environment.Pep425Tag), ShouldBeGreaterThan, 0)
				So(v.Environment.Spec, ShouldNotBeNil)
				So(len(v.Environment.Spec.Wheel), ShouldEqual, len(config.Spec.Wheel))
				So(v.Environment.Spec.Virtualenv, ShouldNotBeNil)
				So(v.Environment.Spec.PythonVersion, ShouldNotEqual, "")

				return nil
			})
			So(err, ShouldBeNil)

			// We should be able to delete it.
			v, err := config.makeEnv(c, nil)
			So(err, ShouldBeNil)

			So(v.Delete(c), ShouldBeNil)
			So(v.Root, shouldNotExist)
			So(v.lockPath, shouldNotExist)
		})

		Convey(`Testing new environment setup race`, func() {
			const workers = 4

			envs := make([]*vpython.Environment, workers)
			err := parallel.FanOutIn(func(taskC chan<- func() error) {
				for i := 0; i < workers; i++ {
					i := i

					taskC <- func() error {
						return With(c, config, func(c context.Context, v *Env) error {
							// Has successfully loaded an Environment.
							envs[i] = v.Environment

							// Can use the Python interpreter.
							if _, err := v.Interpreter().GetVersion(c); err != nil {
								return err
							}
							return nil
						})
					}
				}
			})
			So(err, ShouldBeNil)

			// All Environments must be equal.
			var archetype *vpython.Environment
			for _, env := range envs {
				if archetype == nil {
					archetype = env
				} else {
					So(env, ShouldResemble, archetype)
				}
			}
		})
	}))
}

func TestVirtualEnv(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		ri   *resolvedInterpreter
	}{
		{"python27", python27},
		{"python3", python3},
	} {
		tc := tc

		t.Run(fmt.Sprintf(`Testing Virtualenv for: %s`, tc.name), func(t *testing.T) {
			testVirtualEnvWith(t, tc.ri)
		})
	}
}

func loadJSON(path string, dst interface{}) error {
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return errors.Annotate(err, "failed to open file").Err()
	}
	if err := json.Unmarshal(content, dst); err != nil {
		return errors.Annotate(err, "failed to unmarshal JSON").Err()
	}
	return nil
}

func shouldNotExist(actual interface{}, expected ...interface{}) string {
	path := actual.(string)
	switch _, err := os.Stat(path); {
	case err == nil:
		return fmt.Sprintf("Path %q should not exist, but it does.", path)
	case os.IsNotExist(err):
		return ""
	default:
		return fmt.Sprintf("Couldn't check if %q exists: %s", path, err)
	}
}
