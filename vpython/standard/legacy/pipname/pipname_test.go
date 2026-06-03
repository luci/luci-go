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

package pipname

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPipHelpers(tT *testing.T) {
	tT.Parallel()

	ftt.Run("Test pip name and version helper utilities", tT, func(t *ftt.Test) {
		t.Run("PipNameFromPackageName", func(t *ftt.Test) {
			cases := [...]struct {
				in, out string
			}{
				{"infra/python/wheels/six-py2_py3/version:2.15.0", "six"},
				{"infra/python/wheels/six-py3/version:2.15.0", "six"},
				{"infra/python/wheels/cryptography/linux-amd64", "cryptography"},
				{"infra/python/wheels/numpy-py3", "numpy"},
				{"infra/python/wheels/requests_py2_py3", "requests"},
				{"my-package", "my-package"},
				{"infra/python/wheels/my._-package", "my-package"},
				{"infra/python/wheels/complex---name", "complex-name"},
				{"custom/third_party/requests", "requests"},
				{"custom/third_party/numpy/linux-amd64", "linux-amd64"},
				{"custom/third_party/numpy/${vpython_platform}", "numpy"},
			}
			for _, c := range cases {
				assert.Loosely(t, PipNameFromPackageName(c.in), should.Equal(c.out))
			}
		})

		t.Run("PipVersionFromPackageVersion", func(t *ftt.Test) {
			cases := [...]struct {
				in, out string
			}{
				{"version:1.15.0", "1.15.0"},
				{"version:2@1.15.0.chromium.1", "1.15.0+chromium.1"},
				{"my-hash", "my-hash"},
				{"version:1.2.0.chromium.1", "1.2.0+chromium.1"},
				{"version:1.2.0+chromium.1", "1.2.0+chromium.1"},
				{"version:1.2.0.post1.chromium.1", "1.2.0.post1+chromium.1"},
				{"version:1.2.0.google.2", "1.2.0+google.2"},
				{"version:1.2.0-hash", "1.2.0+hash"},
				{"version:1.2.0.c1", "1.2.0.c1"},
				{"version:1.2.0.c", "1.2.0.c"},
				{"version:1.2.0.rc1", "1.2.0.rc1"},
				{"version:2@1.pre1.re", "1.pre1+re"},
				{"version:2.5.6-5c85ed3d46137b17da04c59bcd805ee5", "2.5.6+5c85ed3d46137b17da04c59bcd805ee5"},
				{"version:2.5.6-0a1b2c3d", "2.5.6+0a1b2c3d"},
				{"version:0.39b0", "0.39b0"},
				{"version:1.2.0a1", "1.2.0a1"},
			}
			for _, c := range cases {
				assert.Loosely(t, PipVersionFromPackageVersion(c.in), should.Equal(c.out))
			}
		})
	})
}
