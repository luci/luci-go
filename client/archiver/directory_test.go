// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package archiver

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/client/internal/common"
	"github.com/luci/luci-go/common/data/text/units"
	"github.com/luci/luci-go/common/isolated"
	"github.com/luci/luci-go/common/isolatedclient"
	"github.com/luci/luci-go/common/isolatedclient/isolatedfake"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalkBadRegexp(t *testing.T) {
	Convey(`A bad regexp should fail when walking a directory.`, t, func() {
		ch := make(chan *walkItem)
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(ch)
			walk("inexistent", []string{"a["}, ch)
		}()
		item := <-ch
		So(item, ShouldResemble, &walkItem{err: errors.New("bad blacklist pattern \"a[\"")})
		item, ok := <-ch
		So(item, ShouldBeNil)
		So(ok, ShouldBeFalse)
		wg.Wait()
	})
}

func TestPushDirectory(t *testing.T) {
	// Uploads a real directory. 2 times the same file.
	t.Parallel()
	emptyContext := context.Background()

	Convey(`Pushing a real directory should upload the directory.`, t, func() {
		server := isolatedfake.New()
		ts := httptest.NewServer(server)
		defer ts.Close()
		a := New(emptyContext, isolatedclient.New(nil, nil, ts.URL, isolatedclient.DefaultNamespace, nil, nil), nil)

		// Setup temporary directory.
		tmpDir, err := ioutil.TempDir("", "archiver")
		So(err, ShouldBeNil)
		defer func() {
			if err := os.RemoveAll(tmpDir); err != nil {
				t.Fail()
			}
		}()
		baseDir := filepath.Join(tmpDir, "base")
		ignoredDir := filepath.Join(tmpDir, "ignored1")
		So(os.Mkdir(baseDir, 0700), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(baseDir, "bar"), []byte("foo"), 0600), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(baseDir, "bar_dupe"), []byte("foo"), 0600), ShouldBeNil)
		if !common.IsWindows() {
			So(os.Symlink("bar", filepath.Join(baseDir, "link")), ShouldBeNil)
		}
		So(ioutil.WriteFile(filepath.Join(baseDir, "ignored2"), []byte("ignored"), 0600), ShouldBeNil)
		So(os.Mkdir(ignoredDir, 0700), ShouldBeNil)
		So(ioutil.WriteFile(filepath.Join(ignoredDir, "really"), []byte("ignored"), 0600), ShouldBeNil)

		item := PushDirectory(a, tmpDir, "", []string{"ignored1", filepath.Join("*", "ignored2")})
		So(item.DisplayName, ShouldResemble, filepath.Base(tmpDir)+".isolated")
		item.WaitForHashed()
		So(a.Close(), ShouldBeNil)

		mode := 0600
		if common.IsWindows() {
			mode = 0666
		}
		isolatedData := isolated.Isolated{
			Algo: "sha-1",
			Files: map[string]isolated.File{
				filepath.Join("base", "bar"):      isolated.BasicFile("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33", mode, 3),
				filepath.Join("base", "bar_dupe"): isolated.BasicFile("0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33", mode, 3),
			},
			Version: isolated.IsolatedFormatVersion,
		}
		if !common.IsWindows() {
			isolatedData.Files[filepath.Join("base", "link")] = isolated.SymLink("bar")
		}
		encoded, err := json.Marshal(isolatedData)
		So(err, ShouldBeNil)
		isolatedEncoded := string(encoded) + "\n"
		isolatedHash := isolated.HashBytes([]byte(isolatedEncoded))

		expected := map[string]string{
			"0beec7b5ea3f0fdbc95d0dd47f3c5bc275da8a33": "foo",
			string(isolatedHash):                       isolatedEncoded,
		}
		actual := map[string]string{}
		for k, v := range server.Contents() {
			actual[string(k)] = string(v)
		}
		So(actual, ShouldResemble, expected)
		So(item.Digest(), ShouldResemble, isolatedHash)

		stats := a.Stats()
		So(stats.TotalHits(), ShouldResemble, 0)
		// There're 3 cache misses even if the same content is looked up twice.
		So(stats.TotalMisses(), ShouldResemble, 3)
		So(stats.TotalBytesHits(), ShouldResemble, units.Size(0))
		So(stats.TotalBytesPushed(), ShouldResemble, units.Size(3+3+len(isolatedEncoded)))

		So(server.Error(), ShouldBeNil)
	})
}
