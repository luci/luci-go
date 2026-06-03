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
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.chromium.org/luci/vpython/standard"
)

func BenchmarkSyncLockfileCheck(b *testing.B) {
	ctx := context.Background()
	tempDir := b.TempDir()

	specPath := filepath.Join(tempDir, "vpython.toml")
	lockPath := specPath + ".uv.lock"

	// Create dummy vpython.toml
	os.WriteFile(specPath, []byte(""), 0644)

	// Create a simulated lockfile
	lockContent := `
numpy==1.2.3 \
    --hash=sha256:0c542922586a265e699188e52d5f5ac5ec0dd517e5a1041d90d2bbf23f906058 \
    --hash=sha256:57439f482b36d91b4d0e719f2982abe9da94540a3b3d05c5c1a5e43c7b315c8e
    # via mozlog
requests==2.22.0 \
    --hash=sha256:0c542922586a265e699188e52d5f5ac5ec0dd517e5a1041d90d2bbf23f906058
wheel==0.36.2 \
    --hash=sha256:57439f482b36d91b4d0e719f2982abe9da94540a3b3d05c5c1a5e43c7b315c8e
urllib3==1.25.8
chardet==3.0.4
idna==2.8
certifi==2019.11.28
`
	os.WriteFile(lockPath, []byte(lockContent), 0644)

	spec := &standard.ProjectSpec{
		Dependencies: []string{
			"requests[security]>=2.0.0",
			"numpy",
			"wheel (==0.36.2) ; python_version >= '3.8'",
		},
	}

	for b.Loop() {
		err := SyncLockfile(ctx, specPath, "uv", "python", true, spec)
		if err != nil {
			b.Fatalf("SyncLockfile failed: %v", err)
		}
	}
}
