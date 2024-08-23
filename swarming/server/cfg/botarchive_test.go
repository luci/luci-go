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
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/klauspost/compress/zip"

	"go.chromium.org/luci/cipd/client/cipd/fs"
	"go.chromium.org/luci/cipd/client/cipd/pkg"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	configpb "go.chromium.org/luci/swarming/proto/config"
)

func TestEnsureBotArchiveBuilt(t *testing.T) {
	t.Parallel()

	const (
		testCIPDServer = "https://cipdserver.example.com"
		testCIPDPkg    = "bot/package"
	)
	testTime := time.Date(2044, time.April, 4, 4, 4, 4, 0, time.UTC)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), testTime)
		ctx = memory.Use(ctx)

		build := func(cipdPayload, botConfigPy, serverURL string) BotArchiveInfo {
			cipdClient := &fakeCIPDClient{}
			cipdClient.mockPackage(cipdPayload)
			info, err := ensureBotArchiveBuilt(ctx,
				cipdClient,
				&configpb.BotDeployment_BotPackage{
					Server:  testCIPDServer,
					Pkg:     testCIPDPkg,
					Version: "latest",
				},
				[]byte(botConfigPy),
				"bot-config-rev",
				&EmbeddedBotSettings{ServerURL: serverURL},
				300, // small chunk size to test chunking
			)
			assert.Loosely(t, err, should.BeNil)
			_, err = fetchBotArchive(ctx, info.Chunks)
			assert.Loosely(t, err, should.BeNil)
			return info
		}

		allChunks := func() (chunks []string) {
			q := datastore.NewQuery("BotArchiveChunk").Ancestor(botArchiverStateKey(ctx))
			err := datastore.Run(ctx, q, func(ent *botArchiveChunk) error {
				chunks = append(chunks, ent.Key.StringID())
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			return
		}

		fetchState := func() []botArchive {
			state := botArchiverState{Key: botArchiverStateKey(ctx)}
			assert.Loosely(t, datastore.Get(ctx, &state), should.BeNil)
			return state.Archives
		}

		t.Run("Happy path", func(t *ftt.Test) {
			info := build("v1", "bot_config.py body", "https://swarming.example.com")
			assert.Loosely(t, info, should.Match(BotArchiveInfo{
				Digest: "7a6ec33eb9e48d13194933e27d59bf4960514f1f89c669990ad0b6d85a6c128c",
				Chunks: []string{
					"7a6ec33eb9e48d13194933e27d59bf4960514f1f89c669990ad0b6d85a6c128c:0:300",
					"7a6ec33eb9e48d13194933e27d59bf4960514f1f89c669990ad0b6d85a6c128c:300:600",
					"7a6ec33eb9e48d13194933e27d59bf4960514f1f89c669990ad0b6d85a6c128c:600:760",
				},
				BotConfigHash:     "Y0Cw/oeiiVC47rcv4yUNcyBgP6Q/7qhNzc+mZFIXZEw",
				BotConfigRev:      "bot-config-rev",
				PackageInstanceID: "iid-for-v1",
				PackageServer:     testCIPDServer,
				PacakgeName:       testCIPDPkg,
				PackageVersion:    "latest",
			}))
			blob, err := fetchBotArchive(ctx, info.Chunks)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, len(blob), should.Equal(760))
			assert.Loosely(t, fetchState(), should.HaveLength(1))
			assert.Loosely(t, allChunks(), should.HaveLength(3))

			// Calling again with the same inputs does nothing.
			info2 := build("v1", "bot_config.py body", "https://swarming.example.com")
			assert.That(t, info2, should.Match(info))
			assert.Loosely(t, fetchState(), should.HaveLength(1))
			assert.Loosely(t, allChunks(), should.HaveLength(3))
		})

		t.Run("Reacts to changes to inputs", func(t *ftt.Test) {
			// The base line.
			assert.That(t, build("v1", "cfg1", "s1").Digest,
				should.Equal("761b7f44c27ac969913ef05731887caaa25d139204dba213f92866ef80442486"))

			// Different inputs => different outputs.
			assert.That(t, build("v2", "cfg1", "s1").Digest,
				should.Equal("d232539f24d8065e0c89609251f75448c7d6960d9bbbe3d87b57c3273f2c5691"))
			assert.That(t, build("v1", "cfg2", "s1").Digest,
				should.Equal("66cabc6c8b3671cf444e67bd693eed8afe028526feb1023b5115e35622750391"))
			assert.That(t, build("v1", "cfg1", "s2").Digest,
				should.Equal("25155a8dd3a80124ba82bbe1b8c11ce18362d56b3106f1ffc787cc4f9458b009"))
		})

		t.Run("Cleans up old entries", func(t *ftt.Test) {
			touchTimes := func() (out []time.Time) {
				for _, ar := range fetchState() {
					out = append(out, ar.LastTouched)
				}
				return
			}

			// Create two entries.
			build("v1", "cfg", "s")
			build("v2", "cfg", "s")
			assert.Loosely(t, allChunks(), should.Match([]string{
				"18a124c0fce0bf15fa9947356e4c457ff4daaf7889d65a43f1286f92c8d66362:0:300",
				"18a124c0fce0bf15fa9947356e4c457ff4daaf7889d65a43f1286f92c8d66362:300:600",
				"18a124c0fce0bf15fa9947356e4c457ff4daaf7889d65a43f1286f92c8d66362:600:754",
				"18dc46b20aae1930727d96934572f87e8515889ec2c87b955e90bef26b8e45b2:0:300",
				"18dc46b20aae1930727d96934572f87e8515889ec2c87b955e90bef26b8e45b2:300:600",
				"18dc46b20aae1930727d96934572f87e8515889ec2c87b955e90bef26b8e45b2:600:754",
			}))
			assert.That(t, touchTimes(), should.Match([]time.Time{
				testTime,
				testTime,
			}))

			// One of them is reused very soon. This doesn't bump the last
			// touch time yet.
			tc.Add(botArchiveTouchPeriod - time.Second)
			build("v2", "cfg", "s")
			assert.That(t, touchTimes(), should.Match([]time.Time{
				testTime,
				testTime,
			}))

			// Later the last touch time is bumped.
			tc.Add(2 * time.Second)
			build("v2", "cfg", "s")
			assert.That(t, touchTimes(), should.Match([]time.Time{
				testTime,
				testTime.Add(time.Hour + time.Second),
			}))

			// Much later the unused entry is removed.
			tc.Add(botArchiveExpiry)
			build("v2", "cfg", "s")
			assert.That(t, touchTimes(), should.Match([]time.Time{
				testTime.Add(time.Hour + time.Second + botArchiveExpiry),
			}))
			assert.Loosely(t, allChunks(), should.Match([]string{
				"18dc46b20aae1930727d96934572f87e8515889ec2c87b955e90bef26b8e45b2:0:300",
				"18dc46b20aae1930727d96934572f87e8515889ec2c87b955e90bef26b8e45b2:300:600",
				"18dc46b20aae1930727d96934572f87e8515889ec2c87b955e90bef26b8e45b2:600:754",
			}))
		})
	})
}

func TestBuildBotArchive(t *testing.T) {
	t.Parallel()

	// Note there are no places where buildBotArchive can really fail (the
	// function essentially operates over in-memory files, transforming them from
	// one form into another), so we just test the happy path.

	ftt.Run("Override", t, func(t *ftt.Test) {
		testBotConfig := []byte("I'm totally a python script")

		out, digest, err := buildBotArchive(
			fakeInstance{payload: "some"},
			testBotConfig,
			botArchiveConfig{
				Server:            "https://example.com",
				BotCIPDInstanceID: "some-instance-id",
			},
		)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, digest, should.Equal("f852dc2a2380a3f96389883b5aa4157ec22fd00f8c262c95f8f960df9872f65a"))
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
			"payload:some",
		}))
	})

	ftt.Run("No override", t, func(t *ftt.Test) {
		out, digest, err := buildBotArchive(
			fakeInstance{payload: "some"},
			nil,
			botArchiveConfig{
				Server:            "https://example.com",
				BotCIPDInstanceID: "some-instance-id",
			},
		)
		assert.Loosely(t, err, should.BeNil)
		assert.That(t, digest, should.Equal("9e7f4933c3939563e26f29f38c6fc26b13723bf4841debe42c94d52a12bb787a"))
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
			"payload:some",
		}))
	})
}

// fakeCIPDClient implements cipdClient.
type fakeCIPDClient struct {
	iid string
	pkg pkg.Instance
}

func (c *fakeCIPDClient) mockPackage(payload string) {
	c.iid = fmt.Sprintf("iid-for-%s", payload)
	c.pkg = &fakeInstance{payload: payload}
}

func (c *fakeCIPDClient) ResolveVersion(ctx context.Context, server, cipdpkg, version string) (string, error) {
	return c.iid, nil
}

func (c *fakeCIPDClient) FetchInstance(ctx context.Context, server, cipdpkg, iid string) (pkg.Instance, error) {
	if iid == c.iid {
		return c.pkg, nil
	}
	return nil, errors.Reason("unexpected instance ID").Err()
}

// fakeInstance implements subset of pkg.Instance used by buildBotArchive.
type fakeInstance struct {
	pkg.Instance        // embedded nil, to panic if any other method is called
	payload      string // data of one of the files inside
}

func (f fakeInstance) Files() []fs.File {
	return []fs.File{
		fs.NewTestFile(".cipdpkg/ignored", "ignored", fs.TestFileOpts{}),
		fs.NewTestFile("f1", "data 1", fs.TestFileOpts{}),
		fs.NewTestFile("payload", f.payload, fs.TestFileOpts{}),
		fs.NewTestFile("a/b/c", "data 2", fs.TestFileOpts{}),
		fs.NewTestFile("config/config.json", "original config.json", fs.TestFileOpts{}),
		fs.NewTestFile("config/bot_config.py", "original bot_config.py", fs.TestFileOpts{}),
	}
}

func (f fakeInstance) Close(ctx context.Context, corrupt bool) error {
	return nil
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
