// Copyright 2019 The LUCI Authors.
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

package archiver

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/isolated"
	"go.chromium.org/luci/common/isolatedclient"
)

// fakeFileInfo is a dummy implementation of FileInfo
type fakeFileInfo struct {
	// This first member claims that fakeFileInfo implements os.FileInfo, so that
	// we don't have to implement unused methods. Calls to unimplemented methods
	// will panic.
	os.FileInfo
	size int64
}

func (ffi *fakeFileInfo) Size() int64 {
	return ffi.size
}

func (ffi *fakeFileInfo) Mode() os.FileMode {
	return 0
}

func (ffi *fakeFileInfo) IsDir() bool {
	return false
}

func setUpTarringArchiver(ta *TarringArchiver, largeFileSize int, largeFilePath string, numHashCalls *int) {
	isol := &isolated.Isolated{}
	ta.PrepareToArchive(isol)
	fos := &fakeOS{
		readFiles: map[string]io.Reader{
			largeFilePath: strings.NewReader(strings.Repeat("a", largeFileSize)),
		},
	}
	ta.tracker.lOS = fos
	// Override filesystem walker in tarring_archiver with fake.
	ta.filePathWalk = func(dummy string, walkfunc filepath.WalkFunc) error {
		fileinfo := &fakeFileInfo{size: int64(largeFileSize)}
		return walkfunc(largeFilePath, fileinfo, nil)
	}
	origDoHashFileImpl := ta.tracker.doHashFileImpl
	ta.tracker.doHashFileImpl = func(ut *UploadTracker, path string) (isolated.HexDigest, error) {
		*numHashCalls += 1
		return origDoHashFileImpl(ut, path)
	}
}

func TestFileHashingSharedAcrossArchives(t *testing.T) {
	Convey(`Any given file should only be hashed once, even when it's a dependency of multiple archives`, t, func() {

		// non-nil PushState means don't skip the upload.
		pushState := &isolatedclient.PushState{}
		checker := &fakeChecker{ps: pushState}
		uploader := &fakeUploader{}

		// namespace := isolatedclient.DefaultNamespace
		ta := NewTarringArchiver(checker, uploader)

		// Override filesystem calls in upload_tracker with fake.
		largeFileSize := 10000000
		largeFilePath := "/a/b/foo"

		numHashCalls := 0

		setUpTarringArchiver(ta, largeFileSize, largeFilePath, &numHashCalls)
		ta.Archive([]string{largeFilePath}, "/", []string{}, "isolate1")
		setUpTarringArchiver(ta, largeFileSize, largeFilePath, &numHashCalls)
		ta.Archive([]string{largeFilePath}, "/", []string{}, "isolate2")

		// TODO(https://crbug.com/969162): Fix the caching and then change this
		// assertion to check that numHashCalls == 1.
		So(numHashCalls, ShouldEqual, 2)
	})
}
