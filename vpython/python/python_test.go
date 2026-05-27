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

package python

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/base/workflow"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/standard"
)

func TestUVGenerator(tT *testing.T) {
	tT.Parallel()

	ftt.Run("UV Generators & Builders", tT, func(t *ftt.Test) {
		tempRoot := tT.TempDir()

		binDir := filepath.Join(tempRoot, "bin")
		uvDir := filepath.Join(binDir, "uv")
		err := os.MkdirAll(filepath.Join(uvDir, ".versions"), 0755)
		assert.Loosely(t, err, should.BeNil)
		err = os.WriteFile(filepath.Join(uvDir, ".versions", "uv.cipd_version"), []byte("mock-uv-version-1.0"), 0644)
		assert.Loosely(t, err, should.BeNil)

		t.Run("UVFromPath successfully imports packages", func(t *ftt.Test) {
			gen, err := UVFromPath(uvDir)
			assert.Loosely(t, err, should.BeNil)
			importTarget, ok := gen.(*generators.ImportTargets)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, importTarget.Name, should.Equal("uv"))
			assert.Loosely(t, importTarget.Targets["."].Source, should.Equal(uvDir))
			assert.Loosely(t, importTarget.Targets["."].Version, should.Equal("mock-uv-version-1.0"))
		})

		t.Run("SpecRequirementsGenerator compiles specs correctly", func(t *ftt.Test) {
			spec := &standard.ProjectSpec{
				Dependencies: []string{"six==1.16.0", "requests>=2.0.0"},
			}
			gen := &SpecRequirementsGenerator{Spec: spec}
			derivation, err := gen.Generate(context.Background(), generators.Platforms{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, derivation.Name, should.Equal("vpython_requirements"))
			assert.Loosely(t, string(derivation.GetCopy().GetFiles()["requirements.txt"].GetRaw()), should.Equal("six==1.16.0\nrequests>=2.0.0\n"))
		})

		t.Run("WithUV constructs standard workflow.Generator with correct dependencies", func(t *ftt.Test) {
			cpython := &generators.ImportTargets{Name: "cpython"}
			uvGen := &generators.ImportTargets{Name: "uv"}

			env := Environment{
				Executable: "python3",
				CPython:    cpython,
			}

			spec := &standard.ProjectSpec{
				RequiresPython: ">=3.11",
				Dependencies:   []string{"six==1.16.0"},
			}

			gen := env.WithUV(uvGen, spec)
			workflowGen, ok := gen.(*workflow.Generator)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, workflowGen.Name, should.Equal("uv_venv"))

			// Verify dependencies
			hasCPython := false
			hasUV := false
			hasReq := false
			hasBootstrap := false
			for _, dep := range workflowGen.Dependencies {
				if dep.Generator == cpython {
					hasCPython = true
				}
				if dep.Generator == uvGen {
					hasUV = true
				}
				if _, ok := dep.Generator.(*SpecRequirementsGenerator); ok {
					hasReq = true
				}
				if dep.Generator == bootstrapGen {
					hasBootstrap = true
				}
			}
			assert.Loosely(t, hasCPython, should.BeTrue)
			assert.Loosely(t, hasUV, should.BeTrue)
			assert.Loosely(t, hasReq, should.BeTrue)
			assert.Loosely(t, hasBootstrap, should.BeTrue)

			// Verify Args contain exact bootstrap scripts parameters!
			argsStr := strings.Join(workflowGen.Args, " ")
			assert.Loosely(t, argsStr, should.ContainSubstring("uv_bootstrap.py"))

			assert.Loosely(t, argsStr, should.ContainSubstring("--uv-bin"))
			assert.Loosely(t, argsStr, should.ContainSubstring("--python-bin"))
			assert.Loosely(t, argsStr, should.ContainSubstring(fmt.Sprintf("--req-file %s", filepath.Join("{{.vpython_requirements}}", "requirements.txt"))))
		})
	})
}
