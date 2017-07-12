// Copyright 2014 The LUCI Authors.
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

package local

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
)

// NewTestFile returns File implementation (Symlink == false) backed by a fake
// in-memory data. It is useful in unit tests.
func NewTestFile(name string, data string, executable bool) File {
	return &testFile{
		name:       name,
		data:       data,
		executable: executable,
	}
}

// NewWinTestFile returns a File implementation (Symlink == false, Executable ==
// false) backed by a fake in-memory data with windows attributes. It is useful
// in unit tests.
func NewWinTestFile(name string, data string, attrs WinAttrs) File {
	return &testFile{
		name:     name,
		data:     data,
		winAttrs: attrs,
	}
}

// NewTestSymlink returns File implementation (Symlink == true) backed by a fake
// in-memory data. It is useful in unit tests.
func NewTestSymlink(name string, target string) File {
	return &testFile{
		name:          name,
		symlinkTarget: target,
	}
}

type testFile struct {
	name          string
	data          string
	executable    bool
	symlinkTarget string

	winAttrs WinAttrs
}

func (f *testFile) Name() string       { return f.name }
func (f *testFile) Size() uint64       { return uint64(len(f.data)) }
func (f *testFile) Executable() bool   { return f.executable }
func (f *testFile) Symlink() bool      { return f.symlinkTarget != "" }
func (f *testFile) WinAttrs() WinAttrs { return f.winAttrs }

func (f *testFile) SymlinkTarget() (string, error) {
	if f.symlinkTarget == "" {
		return "", fmt.Errorf("not a symlink: %s", f.Name())
	}
	return f.symlinkTarget, nil
}

func (f *testFile) Open() (io.ReadCloser, error) {
	if f.Symlink() {
		return nil, fmt.Errorf("can't open symlink: %s", f.Name())
	}
	r := bytes.NewReader([]byte(f.data))
	return ioutil.NopCloser(r), nil
}
