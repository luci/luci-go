// Copyright 2024 The LUCI Authors.
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

package cfg

import (
	"bytes"
	"fmt"
	"io"
	"testing"

	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestBuildBotArchive(t *testing.T) {
	t.Parallel()

	// Note there are no places where buildBotArchive can really fail (the
	// function essentially operates over in-memory files, transforming them from
	// one form into another), so we just test the happy path.

	ftt.Run("Override", t, func(t *ftt.Test) {
		testBotConfig := []byte("I'm totally a python script")

		out, digest, err := buildBotArchive(fakeInstance{}, testBotConfig, botArchiveConfig{
			Server:            "https://example.com",
			BotCIPDInstanceID: "some-instance-id",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, digest, should.Equal("1b3f9bd6984d49edc3047bf0cbd613edc976ce42a4059fe50ef87067e0a71eec"))
		assert.That(t, readZip(out), should.Match([]string{
			"a/b/c:data 2",
			"config/bot_config.py:I'm totally a python script",
			`config/config.json:{
  "server": "https://example.com",
  "bot_cipd_instance_id": "some-instance-id",
  "server_version": "",
  "enable_ts_monitoring": false
}`,
			"f1:data 1",
		}))
	})

	ftt.Run("No override", t, func(t *ftt.Test) {
		out, digest, err := buildBotArchive(fakeInstance{}, nil, botArchiveConfig{
			Server:            "https://example.com",
			BotCIPDInstanceID: "some-instance-id",
		})
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, digest, should.Equal("e41426bcd46424cbf9840cf0a6b1a1434b563500ae1417bbe9868699367793cc"))
		assert.That(t, readZip(out), should.Match([]string{
			"a/b/c:data 2",
			"config/bot_config.py:original bot_config.py",
			`config/config.json:{
  "server": "https://example.com",
  "bot_cipd_instance_id": "some-instance-id",
  "server_version": "",
  "enable_ts_monitoring": false
}`,
			"f1:data 1",
		}))
	})
}

// fakeInstance implements subset of pkg.Instance used by buildBotArchive.
type fakeInstance struct {
	pkg.Instance // embedded nil, to panic if any other method is called
}

func (fakeInstance) Files() []fs.File {
	return []fs.File{
		fs.NewTestFile(".cipdpkg/ignored", "ignored", fs.TestFileOpts{}),
		fs.NewTestFile("f1", "data 1", fs.TestFileOpts{}),
		fs.NewTestFile("a/b/c", "data 2", fs.TestFileOpts{}),
		fs.NewTestFile("config/config.json", "original config.json", fs.TestFileOpts{}),
		fs.NewTestFile("config/bot_config.py", "original bot_config.py", fs.TestFileOpts{}),
	}
}

func readZip(body []byte) []string {
	r, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		panic(err)
	}
	var out []string
	for _, f := range r.File {
		src, err := f.Open()
		if err != nil {
			panic(err)
		}
		blob, err := io.ReadAll(src)
		_ = src.Close()
		if err != nil {
			panic(err)
		}
		out = append(out, fmt.Sprintf("%s:%s", f.Name, blob))
	}
	return out
}
