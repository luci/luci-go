// Copyright 2020 The LUCI Authors.
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

package cas

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/chunker"
	"github.com/bazelbuild/remote-apis-sdks/go/pkg/digest"
	. "github.com/smartystreets/goconvey/convey"
)

func TestChunkerDeduper(t *testing.T) {
	Convey(`Basic`, t, func() {
		fooCh := chunker.NewFromBlob([]byte("foo"), chunker.DefaultChunkSize)
		barCh := chunker.NewFromBlob([]byte("bar"), chunker.DefaultChunkSize)
		bazCh := chunker.NewFromBlob([]byte("baz"), chunker.DefaultChunkSize)

		cd := NewChunkerDeduper()
		deduped := cd.Deduplicate([]*chunker.Chunker{fooCh, barCh})
		So(deduped, ShouldResemble, []*chunker.Chunker{fooCh, barCh})

		deduped = cd.Deduplicate([]*chunker.Chunker{bazCh, barCh})
		So(deduped, ShouldResemble, []*chunker.Chunker{bazCh})
	})

	Convey(`Files`, t, func() {
		tmpDir := t.TempDir()
		fooPath := filepath.Join(tmpDir, "foo")
		So(ioutil.WriteFile(fooPath, []byte("foo"), 0600), ShouldBeNil)
		barPath := filepath.Join(tmpDir, "bar")
		So(ioutil.WriteFile(barPath, []byte("bar"), 0600), ShouldBeNil)

		dirPath := filepath.Join(tmpDir, "dir")
		So(os.Mkdir(dirPath, 0700), ShouldBeNil)
		// Same content as fooPath, but at a different path
		foo2Path := filepath.Join(dirPath, "foo2")
		So(ioutil.WriteFile(foo2Path, []byte("foo"), 0600), ShouldBeNil)
		bazPath := filepath.Join(dirPath, "baz")
		So(ioutil.WriteFile(bazPath, []byte("baz"), 0600), ShouldBeNil)

		fooCh := chunkerFromFile(t, fooPath)
		barCh := chunkerFromFile(t, barPath)
		foo2Ch := chunkerFromFile(t, foo2Path)
		bazCh := chunkerFromFile(t, bazPath)

		cd := NewChunkerDeduper()
		deduped := cd.Deduplicate([]*chunker.Chunker{fooCh, barCh})
		So(deduped, ShouldResemble, []*chunker.Chunker{fooCh, barCh})

		deduped = cd.Deduplicate([]*chunker.Chunker{foo2Ch, bazCh})
		So(deduped, ShouldResemble, []*chunker.Chunker{bazCh})
	})
}

func chunkerFromFile(t *testing.T, path string) *chunker.Chunker {
	t.Helper()
	dg, err := digest.NewFromFile(path)
	So(err, ShouldBeNil)
	return chunker.NewFromFile(path, dg, chunker.DefaultChunkSize)
}
