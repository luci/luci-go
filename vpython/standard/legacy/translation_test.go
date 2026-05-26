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

package legacy

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/vpython/api/vpython"
)

func TestLegacySpecTranslation(tT *testing.T) {
	tT.Parallel()

	ftt.Run("Legacy Spec Translation", tT, func(t *ftt.Test) {
		t.Run("Translates basic universal packages without markers", func(t *ftt.Test) {
			spec := &vpython.Spec{
				PythonVersion: "3.11",
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/requests-py3",
						Version: "version:2.31.0",
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project, should.NotBeNil)
			assert.Loosely(t, project.RequiresPython, should.Equal(">=3.11,<3.12"))
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"requests==2.31.0",
			}))
		})

		t.Run("Handles nil package pointers and malformed inputs defensively", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					nil, // Corrupt nil pointer package
					{
						Name:    "infra/python/wheels/requests-py3",
						Version: "version:2.31.0",
						MatchTag: []*vpython.PEP425Tag{
							nil, // Corrupt nil pointer tag
						},
					},
				},
			}
			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"requests==2.31.0",
			}))
		})

		t.Run("Translates basic universal packages with varied Python version formats", func(t *ftt.Test) {
			// Single segment version "3" (generic modern Python 3 target)
			specSingle := &vpython.Spec{PythonVersion: "3"}
			projectSingle, err := TranslateLegacySpec(specSingle)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projectSingle.RequiresPython, should.Equal(">=3.0,<4.0"))

			// Triple segment version "3.11.8" (strict patch-level base requirement)
			specTriple := &vpython.Spec{PythonVersion: "3.11.8"}
			projectTriple, err := TranslateLegacySpec(specTriple)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projectTriple.RequiresPython, should.Equal(">=3.11.8,<3.12"))
		})

		t.Run("Translates packages with single MatchTag (positive selector)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/numpy-py3/${vpython_platform}",
						Version: "version:1.24.0",
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "linux_x86_64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"numpy==1.24.0; sys_platform == 'linux' and platform_machine == 'x86_64'",
			}))
		})

		t.Run("Translates packages with multiple MatchTags (OR condition)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/numpy-py3/${vpython_platform}",
						Version: "version:1.24.0",
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "linux_x86_64"},
							{Platform: "macosx_10_15_x86_64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"numpy==1.24.0; (sys_platform == 'linux' and platform_machine == 'x86_64') or (sys_platform == 'darwin' and platform_machine == 'x86_64')",
			}))
		})

		t.Run("Translates packages with NotMatchTag (negative selector)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/psutil-py3/${vpython_platform}",
						Version: "version:5.9.0",
						NotMatchTag: []*vpython.PEP425Tag{
							{Platform: "win_amd64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"psutil==5.9.0; sys_platform != 'win32' or (platform_machine != 'AMD64' and platform_machine != 'x86_64')",
			}))
		})

		t.Run("Translates packages with both MatchTags and NotMatchTags (combined)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/cryptography-py3/${vpython_platform}",
						Version: "version:40.0.0",
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "linux_x86_64"},
							{Platform: "win_amd64"},
						},
						NotMatchTag: []*vpython.PEP425Tag{
							{Platform: "win32"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"cryptography==40.0.0; ((sys_platform == 'linux' and platform_machine == 'x86_64') or (sys_platform == 'win32' and (platform_machine == 'AMD64' or platform_machine == 'x86_64'))) and (sys_platform != 'win32' or platform_machine != 'x86')",
			}))
		})

		t.Run("Translates partial match tags (omitted fields)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/custom-wheel",
						Version: "version:1.0.0",
						MatchTag: []*vpython.PEP425Tag{
							{Python: "cp311"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"custom-wheel==1.0.0; python_version == '3.11'",
			}))
		})

		t.Run("Strips universal wheel suffixes and normalizes names correctly", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/complex_name.Normalised-py2_py3",
						Version: "version:2@1.2.3-custom-tag-name",
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"complex-name-normalised==1.2.3+custom.tag.name",
			}))
		})

		t.Run("Translates modern Manylinux tags (manylinux2014)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/cryptography-py3/${vpython_platform}",
						Version: "version:40.0.0",
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "manylinux2014_x86_64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"cryptography==40.0.0; sys_platform == 'linux' and platform_machine == 'x86_64'",
			}))
		})

		t.Run("Translates PEP 600 Manylinux tags (manylinux_2_17)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/cryptography-py3/${vpython_platform}",
						Version: "version:40.0.0",
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "manylinux_2_17_aarch64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"cryptography==40.0.0; sys_platform == 'linux' and platform_machine == 'aarch64'",
			}))
		})

		t.Run("Translates packages with Windows ARM64 tags (win_arm64)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/cryptography-py3/${vpython_platform}",
						Version: "version:40.0.0",
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "win_arm64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"cryptography==40.0.0; sys_platform == 'win32' and platform_machine == 'ARM64'",
			}))
		})

		t.Run("Translates packages with empty/unspecified version string", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/requests-py3",
						Version: "", // Empty/unspecified version
						MatchTag: []*vpython.PEP425Tag{
							{Platform: "linux_x86_64"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"requests; sys_platform == 'linux' and platform_machine == 'x86_64'",
			}))
		})

		t.Run("Translates future CPython version tags (cp410)", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/custom-wheel",
						Version: "version:1.0.0",
						MatchTag: []*vpython.PEP425Tag{
							{Python: "cp410"},
						},
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"custom-wheel==1.0.0; python_version == '4.10'",
			}))
		})

		t.Run("Handles segment collision index safely during local version parsing", func(t *ftt.Test) {
			spec := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "infra/python/wheels/complex-wheel",
						Version: "version:2@1.pre1.re", // Collision: "re" is a trailing segment, but is a substring of "pre"
					},
				},
			}

			project, err := TranslateLegacySpec(spec)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, project.Dependencies, should.Match([]string{
				"complex-wheel==1.pre1+re",
			}))
		})

		t.Run("Climbs folder paths to resolve custom package names (non-wheels prefix)", func(t *ftt.Test) {
			// Case A: Custom standard path with zero variables
			specPath := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "custom/third_party/requests",
						Version: "version:2.31.0",
					},
				},
			}
			projectPath, err := TranslateLegacySpec(specPath)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projectPath.Dependencies, should.Match([]string{
				"requests==2.31.0",
			}))

			// Case B: Custom path ending in platform subfolder
			specPlatform := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "custom/third_party/numpy/linux-amd64",
						Version: "version:1.24.0",
					},
				},
			}
			projectPlatform, err := TranslateLegacySpec(specPlatform)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projectPlatform.Dependencies, should.Match([]string{
				"linux-amd64==1.24.0",
			}))

			// Case C: Custom path with trailing dynamic platform template variables
			specVar := &vpython.Spec{
				Wheel: []*vpython.Spec_Package{
					{
						Name:    "custom/third_party/numpy/${vpython_platform}",
						Version: "version:1.24.0",
					},
				},
			}
			projectVar, err := TranslateLegacySpec(specVar)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, projectVar.Dependencies, should.Match([]string{
				"numpy==1.24.0",
			}))
		})

		t.Run("Translates PEP 425 platform tags for all supported architectures and machines", func(t *ftt.Test) {
			cases := [...]struct {
				platform string
				expected string
			}{
				// Linux / Manylinux
				{"linux_x86_64", "sys_platform == 'linux' and platform_machine == 'x86_64'"},
				{"manylinux2014_x86_64", "sys_platform == 'linux' and platform_machine == 'x86_64'"},
				{"manylinux_2_17_aarch64", "sys_platform == 'linux' and platform_machine == 'aarch64'"},
				{"linux_i686", "sys_platform == 'linux' and (platform_machine == 'i386' or platform_machine == 'i686')"},
				{"manylinux_2_12_i386", "sys_platform == 'linux' and (platform_machine == 'i386' or platform_machine == 'i686')"},
				{"linux_armv7l", "sys_platform == 'linux' and platform_machine == 'armv7l'"},
				{"linux_riscv64", "sys_platform == 'linux' and platform_machine == 'riscv64'"},
				{"linux_generic", "sys_platform == 'linux'"},

				// macOS / Darwin
				{"macosx_10_15_x86_64", "sys_platform == 'darwin' and platform_machine == 'x86_64'"},
				{"macosx_11_0_intel", "sys_platform == 'darwin' and platform_machine == 'x86_64'"},
				{"macosx_10_9_universal", "sys_platform == 'darwin' and platform_machine == 'x86_64'"},
				{"macosx_10_6_fat64", "sys_platform == 'darwin' and platform_machine == 'x86_64'"},
				{"macosx_11_0_arm64", "sys_platform == 'darwin' and platform_machine == 'arm64'"},
				{"macosx_generic", "sys_platform == 'darwin'"},

				// Windows
				{"win32", "sys_platform == 'win32' and platform_machine == 'x86'"},
				{"win_amd64", "sys_platform == 'win32' and (platform_machine == 'AMD64' or platform_machine == 'x86_64')"},
				{"win_arm64", "sys_platform == 'win32' and platform_machine == 'ARM64'"},
				{"win_generic", "sys_platform == 'win32'"},
			}

			for _, tc := range cases {
				t.Run(tc.platform, func(t *ftt.Test) {
					spec := &vpython.Spec{
						Wheel: []*vpython.Spec_Package{
							{
								Name:    "infra/python/wheels/package-py3",
								Version: "version:1.0.0",
								MatchTag: []*vpython.PEP425Tag{
									{Platform: tc.platform},
								},
							},
						},
					}
					project, err := TranslateLegacySpec(spec)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, project.Dependencies, should.Match([]string{
						"package==1.0.0; " + tc.expected,
					}))
				})
			}
		})
	})
}
