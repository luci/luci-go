// Copyright 2026 The LUCI Authors.
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
	"archive/zip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	vpythonAPI "go.chromium.org/luci/vpython/api/vpython"
	"go.chromium.org/luci/vpython/common"
	"go.chromium.org/luci/vpython/wheels"
)

func TestFallback(t *testing.T) {
	// Set bogus AR URL to trigger fallback
	t.Setenv(common.EnvVpythonArUrl, "https://invalid.example.com")

	ftt.Run("Test fallback", t, func(t *ftt.Test) {
		// Arrange
		ctx := memlogger.Use(context.Background())
		ctx = logging.SetLevel(ctx, logging.Debug)

		env := getPythonEnvironment(defaultPythonVersion)
		app := &Application{
			Arguments: []string{
				"-c", "pass",
			},
			PythonExecutable: env.Executable,
		}

		t.Parallel("to CIPD (on-demand fetch)", func(t *ftt.Test) {
			app.VpythonSpec = &vpythonAPI.Spec{
				Wheel: []*vpythonAPI.Spec_Package{
					{Name: "infra/python/wheels/six-py2_py3", Version: "version:1.17.0"},
				},
			}
			ctx = setupApp(ctx, t, app)

			tags := env.Pep425Tags()
			venv := env.WithWheels(wheels.FromSpec(app.VpythonSpec, tags))

			// Act
			buildVENV(ctx, t, app, venv)

			// Assert
			foundARFail := false
			foundCIPDFetch := false
			for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
				if strings.Contains(m.Msg, "Falling back to CIPD") {
					foundARFail = true
				}
				if strings.Contains(m.Msg, "Falling back to on-demand fetch") {
					foundCIPDFetch = true
				}
			}
			if !foundARFail || !foundCIPDFetch {
				for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
					t.Log(m.Msg)
				}
			}
			assert.Loosely(t, foundARFail, should.BeTrue)
			assert.Loosely(t, foundCIPDFetch, should.BeTrue)
		})

		t.Run("itemized fallback with local fake AR server", func(t *ftt.Test) {
			ctx = setupApp(ctx, t, app)

			// Create a valid dummy wheel for 'pkg1' (AR Success)
			pkgWheelPath := filepath.Join(t.TempDir(), "pkg1-1.0.0-py2.py3-none-any.whl")
			createDummyWheel(t, pkgWheelPath, "pkg1", "1.0.0")

			// Start a local HTTP server as Fake Artifact Registry
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "pkg1") {
					if strings.HasSuffix(r.URL.Path, ".whl") {
						http.ServeFile(w, r, pkgWheelPath)
						return
					}
					// Serve simple API HTML
					w.Header().Set("Content-Type", "text/html")
					fmt.Fprintln(w, `<html><body><a href="pkg1-1.0.0-py2.py3-none-any.whl">pkg1-1.0.0-py2.py3-none-any.whl</a></body></html>`)
					return
				}
				w.WriteHeader(http.StatusNotFound)
			}))
			defer ts.Close()

			t.Setenv(common.EnvVpythonArUrl, ts.URL)

			app.VpythonSpec = &vpythonAPI.Spec{
				Wheel: []*vpythonAPI.Spec_Package{
					{Name: "infra/python/wheels/pkg1-py2_py3", Version: "version:1.0.0"}, // AR Success
					{Name: "infra/python/wheels/six-py2_py3", Version: "version:1.15.0"}, // AR Fail, CIPD Success
				},
			}

			tags := env.Pep425Tags()
			venv := env.WithWheels(wheels.FromSpec(app.VpythonSpec, tags))

			// Act
			buildVENV(ctx, t, app, venv)

			// Assert
			foundFailedRequests := false
			foundFilteredLog := false
			for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
				if strings.Contains(m.Msg, "Failed to download") && strings.Contains(m.Msg, "six") {
					foundFailedRequests = true
				}
				if strings.Contains(m.Msg, "Filtering ensure.txt to only download failed wheels") {
					foundFilteredLog = true
				}
			}
			assert.Loosely(t, foundFailedRequests, should.BeTrue)
			assert.Loosely(t, foundFilteredLog, should.BeTrue)
		})

		t.Parallel("itemized fallback with missing mapping.json (heuristic)", func(t *ftt.Test) {
			ctx = setupApp(ctx, t, app)

			// Create a fake wheels directory missing mapping.json and the 'wheels' subdir
			// to force the on-demand fetch logic.
			wheelsSourceDir := t.TempDir()
			os.WriteFile(filepath.Join(wheelsSourceDir, "requirements.txt"), []byte("six==1.17.0\n"), 0644)
			// Create ensure.txt
			os.WriteFile(filepath.Join(wheelsSourceDir, "ensure.txt"), []byte("infra/python/wheels/six-py2_py3 version:1.17.0\n"), 0644)

			wheelsTarget := &generators.ImportTargets{
				Name: "wheels",
				Targets: map[string]generators.ImportTarget{
					".": {Source: wheelsSourceDir, Mode: os.ModeDir},
				},
			}

			venv := env.WithWheels(wheelsTarget)
			ap := actions.NewActionProcessor()
			wheels.MustSetTransformer(app.CIPDCacheDir, ap)

			// Act
			err := app.BuildVENV(ctx, ap, venv)
			assert.Loosely(t, err, should.BeNil)
			defer app.close()

			// Assert
			foundFailedRequests := false
			foundFilteredLog := false
			for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
				if strings.Contains(m.Msg, "Failed to download") && strings.Contains(m.Msg, "six") {
					foundFailedRequests = true
				}
				if strings.Contains(m.Msg, "Filtering ensure.txt to only download failed wheels...") {
					foundFilteredLog = true
				}
			}
			assert.Loosely(t, foundFailedRequests, should.BeTrue)
			assert.Loosely(t, foundFilteredLog, should.BeTrue)
		})

		t.Parallel("from AR to populated CIPD (skip fetch)", func(t *ftt.Test) {
			ctx = setupApp(ctx, t, app)

			// Create a fake populated wheels directory
			wheelsSourceDir := t.TempDir()
			os.Mkdir(filepath.Join(wheelsSourceDir, "wheels"), 0744)
			os.WriteFile(filepath.Join(wheelsSourceDir, "requirements.txt"), []byte("six==1.17.0\n"), 0644)

			populatedWheels := &generators.ImportTargets{
				Name: "wheels",
				Targets: map[string]generators.ImportTarget{
					".": {Source: wheelsSourceDir, Mode: os.ModeDir},
				},
			}

			venv := env.WithWheels(populatedWheels)
			ap := actions.NewActionProcessor()
			wheels.MustSetTransformer(app.CIPDCacheDir, ap)

			// Act. This will error out because the wheel folder is incomplete, but we are not testing this.
			_ = app.BuildVENV(ctx, ap, venv)

			// Assert
			foundARFail := false
			for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
				if strings.Contains(m.Msg, "Falling back to CIPD") {
					foundARFail = true
				}
				if strings.Contains(m.Msg, "Falling back to on-demand fetch") {
					t.Fatal("On-demand fetch reached when it should have stopped at found wheels folder.")
				}
			}
			assert.Loosely(t, foundARFail, should.BeTrue)
		})

		t.Parallel("missing from both AR and CIPD", func(t *ftt.Test) {
			ctx = setupApp(ctx, t, app)

			app.VpythonSpec = &vpythonAPI.Spec{
				Wheel: []*vpythonAPI.Spec_Package{
					{Name: "infra/python/wheels/nonexistent-py2_py3", Version: "version:1.0.0"},
				},
			}

			tags := env.Pep425Tags()
			venv := env.WithWheels(wheels.FromSpec(app.VpythonSpec, tags))

			ap := actions.NewActionProcessor()
			wheels.MustSetTransformer(app.CIPDCacheDir, ap)

			// Act
			err := app.BuildVENV(ctx, ap, venv)

			// Assert
			assert.Loosely(t, err, should.NotBeNil)

			foundFatalLog := false
			for _, m := range logging.Get(ctx).(*memlogger.MemLogger).Messages() {
				if strings.Contains(m.Msg, "FATAL: One or more requirements are missing from BOTH AR and CIPD") {
					foundFatalLog = true
				}
			}
			assert.Loosely(t, foundFatalLog, should.BeTrue)
		})
	})
}

func createDummyWheel(t *ftt.Test, path, name, version string) {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	zw := zip.NewWriter(f)
	defer zw.Close()

	// Add METADATA
	w, err := zw.Create(fmt.Sprintf("%s-%s.dist-info/METADATA", name, version))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(w, "Metadata-Version: 2.1")
	fmt.Fprintf(w, "Name: %s\n", name)
	fmt.Fprintf(w, "Version: %s\n", version)

	// Add WHEEL file
	w, err = zw.Create(fmt.Sprintf("%s-%s.dist-info/WHEEL", name, version))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(w, "Wheel-Version: 1.0")
	fmt.Fprintln(w, "Generator: dummy")
	fmt.Fprintln(w, "Root-Is-Purelib: true")
	fmt.Fprintln(w, "Tag: py3-none-any")

	// Add RECORD file (minimal)
	w, err = zw.Create(fmt.Sprintf("%s-%s.dist-info/RECORD", name, version))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(w, "%s-%s.dist-info/METADATA,,\n", name, version)
	fmt.Fprintf(w, "%s-%s.dist-info/WHEEL,,\n", name, version)
	fmt.Fprintf(w, "%s-%s.dist-info/RECORD,,\n", name, version)
}
