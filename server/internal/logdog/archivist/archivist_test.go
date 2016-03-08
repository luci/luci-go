// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archivist

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/server/logdog/storage"
	"github.com/luci/luci-go/server/logdog/storage/memory"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

// testServicesClient implements logdog.ServicesClient sufficient for testing
// and instrumentation.
type testServicesClient struct {
	logdog.ServicesClient

	lsCallback func(*logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error)
	asCallback func(*logdog.ArchiveStreamRequest) error
}

func (sc *testServicesClient) LoadStream(c context.Context, req *logdog.LoadStreamRequest, o ...grpc.CallOption) (
	*logdog.LoadStreamResponse, error) {
	if cb := sc.lsCallback; cb != nil {
		return cb(req)
	}
	return nil, errors.New("no callback implemented")
}

func (sc *testServicesClient) ArchiveStream(c context.Context, req *logdog.ArchiveStreamRequest, o ...grpc.CallOption) (
	*google.Empty, error) {
	if cb := sc.asCallback; cb != nil {
		if err := cb(req); err != nil {
			return nil, err
		}
	}
	return &google.Empty{}, nil
}

// testGSClient is a testing implementation of the gsClient interface.
//
// It does not actually retain any of the written data, since that level of
// testing is done in the archive package.
type testGSClient struct {
	sync.Mutex
	gs.Client

	// objs is a map of filename to "write amount". The write amount is the
	// cumulative amount of data written to the Writer for a given GS path.
	objs   map[string]int64
	closed bool

	closeErr     error
	newWriterErr func(w *testGSWriter) error
	deleteErr    func(string, string) error
}

func (c *testGSClient) NewWriter(bucket, relpath string) (gs.Writer, error) {
	w := testGSWriter{
		client: c,
		path:   c.url(bucket, relpath),
	}
	if c.newWriterErr != nil {
		if err := c.newWriterErr(&w); err != nil {
			return nil, err
		}
	}
	return &w, nil
}

func (c *testGSClient) Close() error {
	if c.closed {
		panic("double close")
	}
	if err := c.closeErr; err != nil {
		return err
	}
	c.closed = true
	return nil
}

func (c *testGSClient) Delete(bucket, relpath string) error {
	if c.deleteErr != nil {
		if err := c.deleteErr(bucket, relpath); err != nil {
			return err
		}
	}

	c.Lock()
	defer c.Unlock()

	delete(c.objs, c.url(bucket, relpath))
	return nil
}

func (c *testGSClient) url(bucket, relpath string) string {
	return fmt.Sprintf("gs://%s/%s", bucket, relpath)
}

type testGSWriter struct {
	client *testGSClient

	path       string
	closed     bool
	writeCount int64

	writeErr error
	closeErr error
}

func (w *testGSWriter) Write(d []byte) (int, error) {
	if err := w.writeErr; err != nil {
		return 0, err
	}

	if w.client.objs == nil {
		w.client.objs = make(map[string]int64)
	}
	w.client.objs[w.path] += int64(len(d))
	w.writeCount += int64(len(d))
	return len(d), nil
}

func (w *testGSWriter) Close() error {
	if w.closed {
		panic("double close")
	}
	if err := w.closeErr; err != nil {
		return err
	}
	w.closed = true
	return nil
}

func (w *testGSWriter) Count() int64 {
	return w.writeCount
}

func TestHandleArchive(t *testing.T) {
	t.Parallel()

	Convey(`A testing archive setup`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)

		st := memory.Storage{}
		gsc := testGSClient{}

		// Set up our test log stream.
		desc := logpb.LogStreamDescriptor{
			Prefix:        "testing",
			Name:          "foo",
			BinaryFileExt: "bin",
		}
		descBytes, err := proto.Marshal(&desc)
		if err != nil {
			panic(err)
		}

		// Utility function to add a log entry for "ls".
		addTestEntry := func(idxs ...int) {
			for _, v := range idxs {
				le := logpb.LogEntry{
					PrefixIndex: uint64(v),
					StreamIndex: uint64(v),
					Content: &logpb.LogEntry_Text{&logpb.Text{
						Lines: []*logpb.Text_Line{
							{
								Value:     fmt.Sprintf("line #%d", v),
								Delimiter: "\n",
							},
						},
					}},
				}

				d, err := proto.Marshal(&le)
				if err != nil {
					panic(err)
				}

				err = st.Put(&storage.PutRequest{
					Path:  desc.Path(),
					Index: types.MessageIndex(v),
					Value: d,
				})
				if err != nil {
					panic(err)
				}

				// Advance the time for each log entry.
				tc.Add(time.Second)
			}
		}

		// Set up our test Coordinator client stubs.
		stream := logdog.LoadStreamResponse{
			State: &logdog.LogStreamState{
				Path:          string(desc.Path()),
				ProtoVersion:  logpb.Version,
				TerminalIndex: -1,
				Archived:      false,
				Purged:        false,
			},
			Desc: descBytes,
		}

		var archiveRequest *logdog.ArchiveStreamRequest
		sc := testServicesClient{
			lsCallback: func(req *logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
				return &stream, nil
			},
			asCallback: func(req *logdog.ArchiveStreamRequest) error {
				archiveRequest = req
				return nil
			},
		}

		ar := Archivist{
			Service:  &sc,
			Storage:  &st,
			GSClient: &gsc,
			GSBase:   gs.Path("gs://archive-test/path/to/archive/"), // Extra slashes to test concatenation.
		}

		task := logdog.ArchiveTask{
			Path:     stream.State.Path,
			Complete: true,
		}

		gsURL := func(p string) string {
			return fmt.Sprintf("gs://archive-test/path/to/archive/%s/%s", desc.Path(), p)
		}

		// hasStreams can be called to check that the retained archiveRequest had
		// data sizes for the named archive stream types.
		//
		// After checking, the values are set to zero. This allows us to use
		// ShouldEqual without hardcoding specific archival sizes into the results.
		hasStreams := func(log, index, data bool) bool {
			So(archiveRequest, ShouldNotBeNil)
			if (log && archiveRequest.StreamSize <= 0) ||
				(index && archiveRequest.IndexSize <= 0) ||
				(data && archiveRequest.DataSize <= 0) {
				return false
			}

			archiveRequest.StreamSize = 0
			archiveRequest.IndexSize = 0
			archiveRequest.DataSize = 0
			return true
		}

		Convey(`Will fail to archive if the specified stream state could not be loaded.`, func() {
			sc.lsCallback = func(*logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
				return nil, errors.New("does not exist")
			}
			So(ar.Archive(c, &task), ShouldErrLike, "does not exist")
		})

		Convey(`Will refrain from archiving if the stream is already archived.`, func() {
			stream.State.Archived = true
			So(ar.Archive(c, &task), ShouldBeNil)
			So(archiveRequest, ShouldBeNil)
		})

		Convey(`Will refrain from archiving if the stream is purged.`, func() {
			stream.State.Purged = true
			So(ar.Archive(c, &task), ShouldBeNil)
			So(archiveRequest, ShouldBeNil)
		})

		// Weird case: the log has been marked for archival, has not been
		// terminated, and is within its completeness delay. This task will not
		// have been dispatched by our archive cron, but let's assert that it
		// behaves correctly regardless.
		Convey(`Will succeed if the log stream had no entries and no terminal index.`, func() {
			So(ar.Archive(c, &task), ShouldBeNil)

			So(hasStreams(true, true, false), ShouldBeTrue)
			So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
				Path:          task.Path,
				Complete:      true,
				TerminalIndex: -1,

				StreamUrl: gsURL("logstream.entries"),
				IndexUrl:  gsURL("logstream.index"),
				DataUrl:   gsURL("data.bin"),
			})
		})

		Convey(`With terminal index "3"`, func() {
			stream.State.TerminalIndex = 3

			Convey(`Will fail if the log stream had a terminal index and no entries.`, func() {
				So(ar.Archive(c, &task), ShouldErrLike, "stream finished short of terminal index")
			})

			Convey(`Will fail to archive {0, 1, 2, 4} (incomplete).`, func() {
				addTestEntry(0, 1, 2, 4)
				So(ar.Archive(c, &task), ShouldErrLike, "non-contiguous log stream")
			})

			Convey(`Will successfully archive {0, 1, 2, 3, 4}, stopping at the terminal index.`, func() {
				addTestEntry(0, 1, 2, 3, 4)
				So(ar.Archive(c, &task), ShouldBeNil)

				So(hasStreams(true, true, true), ShouldBeTrue)
				So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
					Path:          task.Path,
					Complete:      true,
					TerminalIndex: 3,

					StreamUrl: gsURL("logstream.entries"),
					IndexUrl:  gsURL("logstream.index"),
					DataUrl:   gsURL("data.bin"),
				})
			})
		})

		Convey(`When not enforcing stream completeness`, func() {
			task.Complete = false

			Convey(`With no terminal index`, func() {
				Convey(`Will successfully archive if there are no entries.`, func() {
					So(ar.Archive(c, &task), ShouldBeNil)

					So(hasStreams(true, true, false), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Path:          task.Path,
						Complete:      true,
						TerminalIndex: -1,

						StreamUrl: gsURL("logstream.entries"),
						IndexUrl:  gsURL("logstream.index"),
						DataUrl:   gsURL("data.bin"),
					})
				})

				Convey(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index.`, func() {
					addTestEntry(0, 1, 2, 4)
					So(ar.Archive(c, &task), ShouldBeNil)

					So(hasStreams(true, true, true), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Path:          task.Path,
						Complete:      false,
						TerminalIndex: 4,

						StreamUrl: gsURL("logstream.entries"),
						IndexUrl:  gsURL("logstream.index"),
						DataUrl:   gsURL("data.bin"),
					})
				})
			})

			Convey(`With terminal index 3`, func() {
				stream.State.TerminalIndex = 3

				Convey(`Will successfully archive if there are no entries.`, func() {
					So(ar.Archive(c, &task), ShouldBeNil)

					So(hasStreams(true, true, false), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Path:          task.Path,
						Complete:      true,
						TerminalIndex: -1,

						StreamUrl: gsURL("logstream.entries"),
						IndexUrl:  gsURL("logstream.index"),
						DataUrl:   gsURL("data.bin"),
					})
				})

				Convey(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index to 2.`, func() {
					addTestEntry(0, 1, 2, 4)
					So(ar.Archive(c, &task), ShouldBeNil)

					So(hasStreams(true, true, true), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Path:          task.Path,
						Complete:      false,
						TerminalIndex: 2,

						StreamUrl: gsURL("logstream.entries"),
						IndexUrl:  gsURL("logstream.index"),
						DataUrl:   gsURL("data.bin"),
					})
				})
			})
		})

		// Simulate failures during the various stream generation operations.
		Convey(`Stream generation failures`, func() {
			stream.State.TerminalIndex = 3
			addTestEntry(0, 1, 2, 3)

			for _, failName := range []string{"/logstream.entries", "/logstream.index", "/data.bin"} {
				for _, testCase := range []struct {
					name  string
					setup func()
				}{
					{"delete failure", func() {
						gsc.deleteErr = func(b, p string) error {
							if strings.HasSuffix(p, failName) {
								return errors.New("test error")
							}
							return nil
						}
					}},

					{"writer create failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(w.path, failName) {
								return errors.New("test error")
							}
							return nil
						}
					}},

					{"write failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(w.path, failName) {
								w.writeErr = errors.New("test error")
							}
							return nil
						}
					}},

					{"close failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(w.path, failName) {
								w.closeErr = errors.New("test error")
							}
							return nil
						}
					}},

					{"delete on fail failure (double-failure)", func() {
						failed := false

						// Simulate a write failure. This is the error that will actually
						// be returned.
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(w.path, failName) {
								w.writeErr = errors.New("test error")
							}
							return nil
						}

						// This will trigger twice per stream, once on create and once on
						// cleanup after the write fails. We want to return an error in
						// the latter case.
						gsc.deleteErr = func(b, p string) error {
							if strings.HasSuffix(p, failName) {
								if failed {
									return errors.New("other error")
								}

								// First delete (on create).
								failed = true
							}
							return nil
						}
					}},
				} {
					Convey(fmt.Sprintf(`Can handle %s for %s`, testCase.name, failName), func() {
						testCase.setup()
						So(ar.Archive(c, &task), ShouldErrLike, "test error")
					})
				}
			}
		})
	})
}
