// Copyright 2016 The LUCI Authors.
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

package archive

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"

	cloudStorage "cloud.google.com/go/storage"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/archive"
	"go.chromium.org/luci/logdog/common/renderer"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/memory"
)

const (
	testIndexPath  = gs.Path("gs://+/index")
	testStreamPath = gs.Path("gs://+/stream")
)

type logStreamGenerator struct {
	lines []string

	indexBuf  bytes.Buffer
	streamBuf bytes.Buffer
}

func (g *logStreamGenerator) lineFromEntry(e *storage.Entry) string {
	le, err := e.GetLogEntry()
	if err != nil {
		panic(err)
	}

	text := le.GetText()
	if text == nil || len(text.Lines) != 1 {
		panic(fmt.Errorf("bad generated log entry: %#v", le))
	}
	return string(text.Lines[0].Value)
}

func (g *logStreamGenerator) generate(lines ...string) {
	logEntries := make([]*logpb.LogEntry, len(lines))
	for i, line := range lines {
		logEntries[i] = &logpb.LogEntry{
			PrefixIndex: uint64(i),
			StreamIndex: uint64(i),
			Content: &logpb.LogEntry_Text{
				Text: &logpb.Text{
					Lines: []*logpb.Text_Line{
						{Value: []byte(line), Delimiter: "\n"},
					},
				},
			},
		}
	}

	g.lines = lines
	g.indexBuf.Reset()
	g.streamBuf.Reset()
	src := renderer.StaticSource(logEntries)
	err := archive.Archive(archive.Manifest{
		Desc: &logpb.LogStreamDescriptor{
			Prefix: "prefix",
			Name:   "name",
		},
		Source:      &src,
		LogWriter:   &g.streamBuf,
		IndexWriter: &g.indexBuf,
	})
	if err != nil {
		panic(err)
	}
}

func (g *logStreamGenerator) pruneIndexHints() {
	g.modIndex(func(idx *logpb.LogIndex) {
		idx.LastPrefixIndex = 0
		idx.LastStreamIndex = 0
		idx.LogEntryCount = 0
	})
}

func (g *logStreamGenerator) sparseIndex(indices ...uint64) {
	idxMap := make(map[uint64]struct{}, len(indices))
	for _, i := range indices {
		idxMap[i] = struct{}{}
	}

	g.modIndex(func(idx *logpb.LogIndex) {
		entries := make([]*logpb.LogIndex_Entry, 0, len(idx.Entries))
		for _, entry := range idx.Entries {
			if _, ok := idxMap[entry.StreamIndex]; ok {
				entries = append(entries, entry)
			}
		}
		idx.Entries = entries
	})
}

func (g *logStreamGenerator) modIndex(fn func(*logpb.LogIndex)) {
	var index logpb.LogIndex
	if err := proto.Unmarshal(g.indexBuf.Bytes(), &index); err != nil {
		panic(err)
	}
	fn(&index)
	data, err := proto.Marshal(&index)
	if err != nil {
		panic(err)
	}
	g.indexBuf.Reset()
	if _, err := g.indexBuf.Write(data); err != nil {
		panic(err)
	}
}

type errReader struct {
	io.Reader
	err error
}

func (r *errReader) Read(d []byte) (int, error) {
	if r.err != nil {
		return 0, r.err
	}
	return r.Reader.Read(d)
}

type fakeGSClient struct {
	gs.Client

	index  []byte
	stream []byte

	closed bool

	err       error
	indexErr  error
	streamErr error
}

func (c *fakeGSClient) assertNotClosed() {
	if c.closed {
		panic(errors.New("client is closed"))
	}
}

func (c *fakeGSClient) load(g *logStreamGenerator) {
	c.index = append([]byte{}, g.indexBuf.Bytes()...)
	c.stream = append([]byte{}, g.streamBuf.Bytes()...)
}

func (c *fakeGSClient) Close() error {
	c.assertNotClosed()
	c.closed = true
	return nil
}

func (c *fakeGSClient) NewReader(p gs.Path, offset, length int64) (io.ReadCloser, error) {
	c.assertNotClosed()

	// If we have a client-level error, return it.
	if c.err != nil {
		return nil, c.err
	}

	var (
		data      []byte
		readerErr error
	)
	switch p {
	case testIndexPath:
		data, readerErr = c.index, c.indexErr
	case testStreamPath:
		data, readerErr = c.stream, c.streamErr
	default:
		return nil, cloudStorage.ErrObjectNotExist
	}

	if offset >= 0 {
		if offset >= int64(len(data)) {
			offset = int64(len(data))
		}
		data = data[offset:]
	}

	if length >= 0 {
		if length > int64(len(data)) {
			length = int64(len(data))
		}
		data = data[:length]
	}
	return io.NopCloser(&errReader{bytes.NewReader(data), readerErr}), nil
}

func testArchiveStorage(t *testing.T, limit int64) {
	ftt.Run(`A testing archive instance`, t, func(t *ftt.Test) {
		var (
			c      = context.Background()
			client fakeGSClient
			gen    logStreamGenerator
		)
		defer client.Close()

		opts := Options{
			Index:  testIndexPath,
			Stream: testStreamPath,
			Client: &client,
		}
		if limit > 0 {
			opts.Client = &gs.LimitedClient{
				Client:       opts.Client,
				MaxReadBytes: limit,
			}
		}

		st, err := New(opts)
		assert.Loosely(t, err, should.BeNil)
		defer st.Close()

		stImpl := st.(*storageImpl)

		t.Run(`Will fail to Put with ErrReadOnly`, func(t *ftt.Test) {
			assert.Loosely(t, st.Put(c, storage.PutRequest{}), should.Equal(storage.ErrReadOnly))
		})

		t.Run(`Given a stream with 5 log entries`, func(t *ftt.Test) {
			gen.generate("foo", "bar", "baz", "qux", "quux")

			// Basic test cases.
			for _, tc := range []struct {
				title string
				mod   func()
			}{
				{`Complete index`, func() {}},
				{`Empty index protobuf`, func() { gen.sparseIndex() }},
				{`No index provided`, func() { stImpl.Index = "" }},
				{`Invalid index path`, func() { stImpl.Index = "does-not-exist" }},
				{`Sparse index with a start and terminal entry`, func() { gen.sparseIndex(0, 2, 4) }},
				{`Sparse index with a terminal entry`, func() { gen.sparseIndex(1, 3, 4) }},
				{`Sparse index missing a terminal entry`, func() { gen.sparseIndex(1, 3) }},
			} {
				t.Run(fmt.Sprintf(`Test Case: %q`, tc.title), func(t *ftt.Test) {
					tc.mod()

					// Run through per-testcase variant set.
					for _, variant := range []struct {
						title string
						mod   func()
					}{
						{"with hints", func() {}},
						{"without hints", func() { gen.pruneIndexHints() }},
					} {
						t.Run(variant.title, func(t *ftt.Test) {
							variant.mod()
							client.load(&gen)

							var entries []string
							collect := func(e *storage.Entry) bool {
								entries = append(entries, gen.lineFromEntry(e))
								return true
							}

							t.Run(`Can Get [0..]`, func(t *ftt.Test) {
								assert.Loosely(t, st.Get(c, storage.GetRequest{}, collect), should.BeNil)
								assert.Loosely(t, entries, should.Match(gen.lines))
							})

							t.Run(`Can Get [1..].`, func(t *ftt.Test) {
								assert.Loosely(t, st.Get(c, storage.GetRequest{Index: 1}, collect), should.BeNil)
								assert.Loosely(t, entries, should.Match(gen.lines[1:]))
							})

							t.Run(`Can Get [1..2].`, func(t *ftt.Test) {
								assert.Loosely(t, st.Get(c, storage.GetRequest{Index: 1, Limit: 2}, collect), should.BeNil)
								assert.Loosely(t, entries, should.Match(gen.lines[1:3]))
							})

							t.Run(`Can Get [5..].`, func(t *ftt.Test) {
								assert.Loosely(t, st.Get(c, storage.GetRequest{Index: 5}, collect), should.BeNil)
								assert.Loosely(t, entries, should.HaveLength(0))
							})

							t.Run(`Can Get [4].`, func(t *ftt.Test) {
								assert.Loosely(t, st.Get(c, storage.GetRequest{Index: 4, Limit: 1}, collect), should.BeNil)
								assert.Loosely(t, entries, should.Match(gen.lines[4:]))
							})

							t.Run(`Can tail.`, func(t *ftt.Test) {
								e, err := st.Tail(c, "", "")
								assert.Loosely(t, err, should.BeNil)
								assert.Loosely(t, gen.lineFromEntry(e), should.Equal(gen.lines[len(gen.lines)-1]))
							})
						})
					}
				})
			}
		})

		// Individual error test cases.
		for _, tc := range []struct {
			title string
			fn    func() error
		}{
			{"Get", func() error { return st.Get(c, storage.GetRequest{}, func(*storage.Entry) bool { return true }) }},
			{"Tail", func() (err error) {
				_, err = st.Tail(c, "", "")
				return
			}},
		} {
			t.Run(fmt.Sprintf("Testing retrieval: %q", tc.title), func(t *ftt.Test) {
				t.Run(`With missing log stream returns ErrDoesNotExist.`, func(t *ftt.Test) {
					stImpl.Stream = "does-not-exist"

					assert.Loosely(t, st.Get(c, storage.GetRequest{}, nil), should.Equal(storage.ErrDoesNotExist))
				})

				t.Run(`With a client error returns that error.`, func(t *ftt.Test) {
					client.err = errors.New("test error")

					assert.Loosely(t, tc.fn(), should.ErrLike(client.err))
				})

				t.Run(`With an index reader error returns that error.`, func(t *ftt.Test) {
					client.indexErr = errors.New("test error")

					assert.Loosely(t, tc.fn(), should.ErrLike(client.indexErr))
				})

				t.Run(`With an stream reader error returns that error.`, func(t *ftt.Test) {
					client.streamErr = errors.New("test error")

					assert.Loosely(t, tc.fn(), should.ErrLike(client.streamErr))
				})

				t.Run(`With junk index data returns an error.`, func(t *ftt.Test) {
					client.index = []byte{0x00}

					assert.Loosely(t, tc.fn(), should.ErrLike("failed to unmarshal index"))
				})

				t.Run(`With junk stream data returns an error.`, func(t *ftt.Test) {
					client.stream = []byte{0x00, 0x01, 0xff}

					assert.Loosely(t, tc.fn(), should.ErrLike("failed to unmarshal"))
				})

				t.Run(`With data entries and a cache, only loads the index once.`, func(t *ftt.Test) {
					var cache memory.Cache
					stImpl.Cache = &cache

					gen.generate("foo", "bar", "baz", "qux", "quux")
					client.load(&gen)

					// Assert that an attempted load will fail with an error. This is so
					// we don't accidentally test something that doesn't follow the path
					// we're intending to follow.
					client.indexErr = errors.New("not using a cache")
					assert.Loosely(t, tc.fn(), should.ErrLike(client.indexErr))

					for i := 0; i < 10; i++ {
						if i == 0 {
							// First time successfully reads the index.
							client.indexErr = nil
						} else {
							// Subsequent attempts to load the index will result in an error.
							// This ensures that if they are successful, it's because we're
							// hitting the cache.
							client.indexErr = errors.New("not using a cache")
						}

						assert.Loosely(t, tc.fn(), should.BeNil)
					}
				})
			})
		}

		t.Run(`Tail with no log entries returns ErrDoesNotExist.`, func(t *ftt.Test) {
			client.load(&gen)

			_, err := st.Tail(c, "", "")
			assert.Loosely(t, err, should.Equal(storage.ErrDoesNotExist))
		})
	})
}

func TestArchiveStorage(t *testing.T) {
	t.Parallel()
	testArchiveStorage(t, -1)
}

func TestArchiveStorageWithLimit(t *testing.T) {
	t.Parallel()
	testArchiveStorage(t, 4)
}
