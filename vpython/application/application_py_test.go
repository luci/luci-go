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

package application

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/exitcode"
	"go.chromium.org/luci/common/system/filesystem"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/python"
	"go.chromium.org/luci/vpython/wheels"
)

const defaultPythonVersion = "3.8"

func testData(filename string) string {
	return filepath.Join("..", "testdata", filename)
}

var testStorageDir string

func getPythonEnvironment(ver string) *python.Environment {
	return map[string]*python.Environment{
		"2.7": {
			Executable: "python",
			CPython:    python.CPythonFromCIPD("version:2@2.7.18.chromium.44"),
			Virtualenv: python.VirtualenvFromCIPD("version:2@16.7.12.chromium.7"),
		},
		"3.8": {
			Executable: "python3",
			CPython:    python.CPython3FromCIPD("version:2@3.8.10.chromium.34"),
			Virtualenv: python.VirtualenvFromCIPD("version:2@16.7.12.chromium.7"),
		},
		"3.11": {
			Executable: "python3",
			CPython:    python.CPython3FromCIPD("version:2@3.11.8.chromium.35"),
			Virtualenv: python.VirtualenvFromCIPD("version:2@20.25.1.chromium.8"),
		},
	}[ver]
}

func setupApp(ctx context.Context, t testing.TB, app *Application) context.Context {
	t.Helper()
	if app.VpythonRoot == "" {
		app.VpythonRoot = testStorageDir
	}

	app.Initialize(ctx)

	assert.Loosely(t, app.ParseEnvs(ctx), should.BeNil, truth.LineContext())
	assert.Loosely(t, app.ParseArgs(ctx), should.BeNil, truth.LineContext())
	ctx = app.SetLogLevel(ctx)
	assert.Loosely(t, app.LoadSpec(ctx), should.BeNil, truth.LineContext())
	return ctx
}

func buildVENV(ctx context.Context, t testing.TB, app *Application, venv generators.Generator) {
	ap := actions.NewActionProcessor()
	wheels.MustSetTransformer(app.CIPDCacheDir, ap)
	assert.Loosely(t, app.BuildVENV(ctx, ap, venv), should.BeNil, truth.LineContext())

	// Release all the resources so the temporary vpython root directory can be
	// removed on Windows.
	app.close()
}

func cmd(t testing.TB, app *Application, env *python.Environment) *exec.Cmd {
	t.Helper()

	ctx := context.Background()
	if env == nil {
		env = getPythonEnvironment(defaultPythonVersion)
	}
	app.PythonExecutable = env.Executable

	ctx = setupApp(ctx, t, app)

	venv := env.WithWheels(wheels.FromSpec(app.VpythonSpec, env.Pep425Tags()))
	buildVENV(ctx, t, app, venv)

	return app.GetExecCommand()
}

func output(c *exec.Cmd) any {
	var out strings.Builder
	c.Stdout = &out
	c.Stderr = &out
	if err := c.Run(); err != nil {
		return errors.Annotate(err, out.String()).Err()
	}
	return strings.TrimSpace(out.String())
}

func TestMain(m *testing.M) {
	reexec := actions.NewReexecRegistry()
	wheels.MustSetExecutor(reexec)
	reexec.Intercept(context.Background())

	var err error
	if testStorageDir, err = os.MkdirTemp("", "vpython-test-"); err != nil {
		panic(err)
	}

	rc := m.Run()

	if err = filesystem.RemoveAll(testStorageDir); err != nil {
		panic(err)
	}

	os.Exit(rc)
}

func TestPythonBasic(t *testing.T) {
	ftt.Run("Test python basic", t, func(t *ftt.Test) {
		var env *python.Environment
		for _, ver := range []string{"2.7", "3.8", "3.11"} {
			t.Run(ver, func(t *ftt.Test) {
				env = getPythonEnvironment(ver)

				t.Run("test bad cwd", func(t *ftt.Test) {
					cwd, err := os.Getwd()
					assert.Loosely(t, err, should.BeNil)
					err = os.Chdir(testData("test_bad_cwd"))
					assert.Loosely(t, err, should.BeNil)

					c := cmd(t, &Application{
						Arguments: []string{
							"bisect.py",
						},
					}, env)
					assert.Loosely(t, output(c), should.Equal("SUCCESS"))

					err = os.Chdir(cwd)
					assert.Loosely(t, err, should.BeNil)
				})

				t.Run("Test exit code", func(t *ftt.Test) {
					c := cmd(t, &Application{
						Arguments: []string{
							"-vpython-spec",
							testData("default.vpython3"),
							testData("test_exit_code.py"),
						},
					}, env)

					err := output(c).(error)
					rc, has := exitcode.Get(err)
					assert.Loosely(t, has, should.BeTrue)
					assert.Loosely(t, rc, should.Equal(42))
				})

				t.Run("Test symlink root", func(t *ftt.Test) {
					if runtime.GOOS == "windows" {
						t.Skip("See https://github.com/pypa/virtualenv/issues/1949")
					}
					symlinkRoot := filepath.Join(t.TempDir(), "link")
					err := os.Symlink(testStorageDir, symlinkRoot)
					assert.Loosely(t, err, should.BeNil)

					c := cmd(t, &Application{
						Arguments: []string{
							"-vpython-spec",
							testData("default.vpython3"),
							"-c",
							"print(123)",
						},
						VpythonRoot: symlinkRoot,
					}, env)

					assert.Loosely(t, output(c), should.Equal("123"))
				})
			})
		}
	})
}

func TestPythonFromPath(t *testing.T) {
	ftt.Run("Test python from path", t, func(t *ftt.Test) {
		ctx := context.Background()
		env := getPythonEnvironment(defaultPythonVersion)

		app := &Application{
			Arguments: []string{
				"-vpython-spec",
				testData("default.vpython3"),
				testData("test_exit_code.py"),
			},
			PythonExecutable: env.Executable,
		}
		ctx = setupApp(ctx, t, app)

		// We are not actually building venv, but this should also work for python
		// package.
		buildVENV(ctx, t, app, env.CPython)

		// Python located at ${CPython}/bin/python3
		dir := filepath.Dir(filepath.Dir(app.PythonExecutable))
		py, err := python.CPythonFromPath(dir, "cpython3")
		assert.Loosely(t, err, should.BeNil)
		env.CPython = py

		// Run actual command
		c := cmd(t, app, env)
		err = output(c).(error)
		rc, has := exitcode.Get(err)
		assert.Loosely(t, has, should.BeTrue)
		assert.Loosely(t, rc, should.Equal(42))
	})
}

func BenchmarkStartup(b *testing.B) {
	c := func() *exec.Cmd {
		return cmd(b, &Application{
			Arguments: []string{
				"-vpython-spec",
				testData("default.vpython3"),
				"-c",
				"print(1)",
			},
		}, nil)
	}
	assert.Loosely(b, output(c()), should.Equal("1"))
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_ = c()
	}
}
