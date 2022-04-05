// Copyright 2022 The LUCI Authors.
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

package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMustGetDarwinMajorVersion(t *testing.T) {
	ver := mustGetDarwinMajorVersion()
	if ver < 14 {
		t.Errorf("major version is too small compared to version supported by infra go toolchain: %d", ver)
	}
}

func TestMakeHardLinkOrClone(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a")
	dst := filepath.Join(dir, "b")
	if err := os.WriteFile(src, []byte("test"), 0o666); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	if err := makeHardLinkOrClone(src, dst); err != nil {
		t.Fatalf("failed to create hardlink")
	}

	buf, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("file is not cloned correctly: %v", err)
	}
	if want := []byte("test"); bytes.Compare(buf, want) != 0 {
		t.Fatalf("hardlinked or cloned file doesn't have expected content: want: %q, got: %q", want, buf)
	}
}
