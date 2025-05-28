// Copyright 2025 The LUCI Authors.
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

package unixsock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestShorten(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("No unix sockets on Windows")
	}

	tmp := t.TempDir()

	longDir := tmp
	for i := 0; i < 20; i++ {
		longDir = filepath.Join(longDir, "0123456789")
	}
	assert.NoErr(t, os.MkdirAll(longDir, 0777))
	assert.That(t, len(longDir), should.BeGreaterThan(len(syscall.RawSockaddrUnix{}.Path)))

	longPath := filepath.Join(longDir, "test.txt")
	longFile, err := os.Create(longPath)
	assert.NoErr(t, err)
	assert.NoErr(t, longFile.Close())

	for _, allowSelfProc := range []bool{false, true} {
		t.Run(fmt.Sprintf("allowSelfProc = %v", allowSelfProc), func(t *testing.T) {
			shortPath, cleanup, err := shortenImpl(longPath, allowSelfProc)
			assert.NoErr(t, err)
			assert.That(t, len(shortPath), should.BeLessThan(len(syscall.RawSockaddrUnix{}.Path)))

			longFI, err := os.Stat(longPath)
			assert.NoErr(t, err)
			shortFI, err := os.Stat(shortPath)
			assert.NoErr(t, err)

			assert.That(t, os.SameFile(longFI, shortFI), should.BeTrue)
			assert.NoErr(t, cleanup())

			// Cleanup actually worked.
			_, err = os.Stat(shortPath)
			assert.That(t, errors.Is(err, os.ErrNotExist), should.BeTrue)
		})
	}
}
