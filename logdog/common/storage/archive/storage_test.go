// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package archive

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"testing"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/common/archive"
	"github.com/luci/luci-go/logdog/common/renderer"
	"github.com/luci/luci-go/logdog/common/storage"
	"github.com/luci/luci-go/logdog/common/storage/memory"

	cloudStorage "cloud.google.com/go/storage"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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
	return text.Lines[0].Value
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
						{Value: line, Delimiter: "\n"},
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
	return ioutil.NopCloser(&errReader{bytes.NewReader(data), readerErr}), nil
}

func testArchiveStorage(t *testing.T, limit int64) {
	Convey(`A testing archive instance`, t, func() {
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

		st, err := New(c, opts)
		So(err, ShouldBeNil)
		defer st.Close()

		stImpl := st.(*storageImpl)

		Convey(`Will fail to configure with ErrReadOnly`, func() {
			So(st.Config(storage.Config{}), ShouldEqual, storage.ErrReadOnly)
		})

		Convey(`Will fail to Put with ErrReadOnly`, func() {
			So(st.Put(storage.PutRequest{}), ShouldEqual, storage.ErrReadOnly)
		})

		Convey(`Given a stream with 5 log entries`, func() {
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
				Convey(fmt.Sprintf(`Test Case: %q`, tc.title), func() {
					tc.mod()

					// Run through per-testcase variant set.
					for _, variant := range []struct {
						title string
						mod   func()
					}{
						{"with hints", func() {}},
						{"without hints", func() { gen.pruneIndexHints() }},
					} {
						Convey(variant.title, func() {
							variant.mod()
							client.load(&gen)

							var entries []string
							collect := func(e *storage.Entry) bool {
								entries = append(entries, gen.lineFromEntry(e))
								return true
							}

							Convey(`Can Get [0..]`, func() {
								So(st.Get(storage.GetRequest{}, collect), ShouldBeNil)
								So(entries, ShouldResemble, gen.lines)
							})

							Convey(`Can Get [1..].`, func() {
								So(st.Get(storage.GetRequest{Index: 1}, collect), ShouldBeNil)
								So(entries, ShouldResemble, gen.lines[1:])
							})

							Convey(`Can Get [1..2].`, func() {
								So(st.Get(storage.GetRequest{Index: 1, Limit: 2}, collect), ShouldBeNil)
								So(entries, ShouldResemble, gen.lines[1:3])
							})

							Convey(`Can Get [5..].`, func() {
								So(st.Get(storage.GetRequest{Index: 5}, collect), ShouldBeNil)
								So(entries, ShouldHaveLength, 0)
							})

							Convey(`Can Get [4].`, func() {
								So(st.Get(storage.GetRequest{Index: 4, Limit: 1}, collect), ShouldBeNil)
								So(entries, ShouldResemble, gen.lines[4:])
							})

							Convey(`Can tail.`, func() {
								e, err := st.Tail("", "")
								So(err, ShouldBeNil)
								So(gen.lineFromEntry(e), ShouldEqual, gen.lines[len(gen.lines)-1])
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
			{"Get", func() error { return st.Get(storage.GetRequest{}, func(*storage.Entry) bool { return true }) }},
			{"Tail", func() (err error) {
				_, err = st.Tail("", "")
				return
			}},
		} {
			Convey(fmt.Sprintf("Testing retrieval: %q", tc.title), func() {
				Convey(`With missing log stream returns ErrDoesNotExist.`, func() {
					stImpl.Stream = "does-not-exist"

					So(st.Get(storage.GetRequest{}, nil), ShouldEqual, storage.ErrDoesNotExist)
				})

				Convey(`With a client error returns that error.`, func() {
					client.err = errors.New("test error")

					So(errors.Unwrap(tc.fn()), ShouldEqual, client.err)
				})

				Convey(`With an index reader error returns that error.`, func() {
					client.indexErr = errors.New("test error")

					So(errors.Unwrap(tc.fn()), ShouldEqual, client.indexErr)
				})

				Convey(`With an stream reader error returns that error.`, func() {
					client.streamErr = errors.New("test error")

					So(errors.Unwrap(tc.fn()), ShouldEqual, client.streamErr)
				})

				Convey(`With junk index data returns an error.`, func() {
					client.index = []byte{0x00}

					So(tc.fn(), ShouldErrLike, "failed to unmarshal index")
				})

				Convey(`With junk stream data returns an error.`, func() {
					client.stream = []byte{0x00, 0x01, 0xff}

					So(tc.fn(), ShouldErrLike, "failed to unmarshal")
				})

				Convey(`With data entries and a cache, only loads the index once.`, func() {
					var cache memory.Cache
					stImpl.Cache = &cache

					gen.generate("foo", "bar", "baz", "qux", "quux")
					client.load(&gen)

					// Assert that an attempted load will fail with an error. This is so
					// we don't accidentally test something that doesn't follow the path
					// we're intending to follow.
					client.indexErr = errors.New("not using a cache")
					So(errors.Unwrap(tc.fn()), ShouldEqual, client.indexErr)

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

						So(tc.fn(), ShouldBeNil)
					}
				})
			})
		}

		Convey(`Tail with no log entries returns ErrDoesNotExist.`, func() {
			client.load(&gen)

			_, err := st.Tail("", "")
			So(err, ShouldEqual, storage.ErrDoesNotExist)
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
