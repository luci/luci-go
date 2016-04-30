// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package archivist

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/services/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
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

// testTask is an instrumentable Task implementation.
type testTask struct {
	task           *logdog.ArchiveTask
	assertLeaseErr error
	assertCount    int
}

func (t *testTask) UniqueID() string {
	return "totally unique ID"
}

func (t *testTask) Task() *logdog.ArchiveTask {
	return t.task
}

func (t *testTask) AssertLease(context.Context) error {
	if err := t.assertLeaseErr; err != nil {
		return err
	}
	t.assertCount++
	return nil
}

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
	objs   map[gs.Path]int64
	closed bool

	closeErr     error
	newWriterErr func(w *testGSWriter) error
	deleteErr    func(gs.Path) error
	renameErr    func(gs.Path, gs.Path) error
}

func (c *testGSClient) NewWriter(p gs.Path) (gs.Writer, error) {
	w := testGSWriter{
		client: c,
		path:   p,
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

func (c *testGSClient) Delete(p gs.Path) error {
	if c.deleteErr != nil {
		if err := c.deleteErr(p); err != nil {
			return err
		}
	}

	c.Lock()
	defer c.Unlock()

	delete(c.objs, p)
	return nil
}

func (c *testGSClient) Rename(src, dst gs.Path) error {
	if c.renameErr != nil {
		if err := c.renameErr(src, dst); err != nil {
			return err
		}
	}

	c.Lock()
	defer c.Unlock()

	c.objs[dst] = c.objs[src]
	delete(c.objs, src)
	return nil
}

type testGSWriter struct {
	client *testGSClient

	path       gs.Path
	closed     bool
	writeCount int64

	writeErr error
	closeErr error
}

func (w *testGSWriter) Write(d []byte) (int, error) {
	if err := w.writeErr; err != nil {
		return 0, err
	}

	w.client.Lock()
	defer w.client.Unlock()

	if w.client.objs == nil {
		w.client.objs = make(map[gs.Path]int64)
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
		project := "test-project"
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
		addTestEntry := func(p string, idxs ...int) {
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

				err = st.Put(storage.PutRequest{
					Project: config.ProjectName(p),
					Path:    desc.Path(),
					Index:   types.MessageIndex(v),
					Values:  [][]byte{d},
				})
				if err != nil {
					panic(err)
				}

				// Advance the time for each log entry.
				tc.Add(time.Second)
			}
		}

		// Set up our testing archival task.
		expired := 10 * time.Minute
		archiveTask := logdog.ArchiveTask{
			Project:        project,
			Path:           string(desc.Path()),
			SettleDelay:    google.NewDuration(10 * time.Second),
			CompletePeriod: google.NewDuration(expired),
			Key:            []byte("random archival key"),
		}
		expired++ // This represents a time PAST CompletePeriod.

		task := &testTask{
			task: &archiveTask,
		}

		// Set up our test Coordinator client stubs.
		stream := logdog.LoadStreamResponse{
			State: &logdog.LogStreamState{
				Path:          archiveTask.Path,
				ProtoVersion:  logpb.Version,
				TerminalIndex: -1,
				Archived:      false,
				Purged:        false,
			},
			Desc: descBytes,

			// Age is ON the expiration threshold, so not expired.
			Age:         archiveTask.CompletePeriod,
			ArchivalKey: archiveTask.Key,
		}

		var archiveRequest *logdog.ArchiveStreamRequest
		var archiveStreamErr error
		sc := testServicesClient{
			lsCallback: func(req *logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
				return &stream, nil
			},
			asCallback: func(req *logdog.ArchiveStreamRequest) error {
				archiveRequest = req
				return archiveStreamErr
			},
		}

		ar := Archivist{
			Service:       &sc,
			Storage:       &st,
			GSClient:      &gsc,
			GSBase:        gs.Path("gs://archive-test/path/to/archive/"),         // Extra slashes to test concatenation.
			GSStagingBase: gs.Path("gs://archive-test-staging/path/to/archive/"), // Extra slashes to test concatenation.
		}

		gsURL := func(project, name string) string {
			return fmt.Sprintf("gs://archive-test/path/to/archive/%s/%s/%s", project, desc.Path(), name)
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

		Convey(`Will return task and fail to archive if the specified stream state could not be loaded.`, func() {
			sc.lsCallback = func(*logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
				return nil, errors.New("does not exist")
			}

			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "does not exist")
			So(ack, ShouldBeFalse)
		})

		Convey(`Will complete task and refrain from archiving if the stream is already archived.`, func() {
			stream.State.Archived = true

			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "log stream is archived")
			So(ack, ShouldBeTrue)
			So(archiveRequest, ShouldBeNil)
		})

		Convey(`Will complete task and refrain from archiving if the stream is purged.`, func() {
			stream.State.Purged = true

			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "log stream is purged")
			So(ack, ShouldBeTrue)
			So(archiveRequest, ShouldBeNil)
		})

		Convey(`Will return task if the stream is younger than its settle delay.`, func() {
			stream.Age = google.NewDuration(time.Second)

			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "log stream is within settle delay")
			So(ack, ShouldBeFalse)
			So(archiveRequest, ShouldBeNil)
		})

		Convey(`Will return task if the log stream doesn't have an archival key yet.`, func() {
			stream.Age = google.NewDuration(expired)
			stream.ArchivalKey = nil

			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "premature archival request")
			So(ack, ShouldBeFalse)
			So(archiveRequest, ShouldBeNil)
		})

		Convey(`Will complete task and refrain from archiving if archival keys dont' match.`, func() {
			stream.Age = google.NewDuration(expired)
			stream.ArchivalKey = []byte("non-matching archival key")

			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "superfluous archival request")
			So(ack, ShouldBeTrue)
			So(archiveRequest, ShouldBeNil)
		})

		// Weird case: the log has been marked for archival, has not been
		//
		// terminated, and is within its completeness delay. This task should not
		// have been dispatched by our expired archive cron, but let's assert that
		// it behaves correctly regardless.
		Convey(`Will refuse to archive a complete stream with no terminal index.`, func() {
			ack, err := ar.archiveTaskImpl(c, task)
			So(err, ShouldErrLike, "completeness required, but stream has no terminal index")
			So(ack, ShouldBeFalse)
		})

		Convey(`With terminal index "3"`, func() {
			stream.State.TerminalIndex = 3

			Convey(`Will fail not ACK a log stream with no entries.`, func() {
				ack, err := ar.archiveTaskImpl(c, task)
				So(err, ShouldEqual, storage.ErrDoesNotExist)
				So(ack, ShouldBeFalse)
			})

			Convey(`Will fail to archive {0, 1, 2, 4} (incomplete).`, func() {
				addTestEntry(project, 0, 1, 2, 4)

				ack, err := ar.archiveTaskImpl(c, task)
				So(err, ShouldErrLike, "missing log entry")
				So(ack, ShouldBeFalse)
			})

			Convey(`Will successfully archive {0, 1, 2, 3, 4}, stopping at the terminal index.`, func() {
				addTestEntry(project, 0, 1, 2, 3, 4)

				ack, err := ar.archiveTaskImpl(c, task)
				So(err, ShouldBeNil)
				So(ack, ShouldBeTrue)

				So(hasStreams(true, true, true), ShouldBeTrue)
				So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
					Project:       project,
					Path:          archiveTask.Path,
					LogEntryCount: 4,
					TerminalIndex: 3,

					StreamUrl: gsURL(project, "logstream.entries"),
					IndexUrl:  gsURL(project, "logstream.index"),
					DataUrl:   gsURL(project, "data.bin"),
				})
			})

			Convey(`When a transient archival error occurs, will not ACK it.`, func() {
				addTestEntry(project, 0, 1, 2, 3, 4)
				gsc.newWriterErr = func(*testGSWriter) error { return errors.WrapTransient(errors.New("test error")) }

				ack, err := ar.archiveTaskImpl(c, task)
				So(err, ShouldErrLike, "test error")
				So(ack, ShouldBeFalse)
			})

			Convey(`When a non-transient archival error occurs`, func() {
				addTestEntry(project, 0, 1, 2, 3, 4)
				archiveErr := errors.New("archive failure error")
				gsc.newWriterErr = func(*testGSWriter) error { return archiveErr }

				Convey(`If remote report returns an error, do not ACK.`, func() {
					archiveStreamErr = errors.New("test error")

					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldErrLike, "test error")
					So(ack, ShouldBeFalse)

					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project: project,
						Path:    archiveTask.Path,
						Error:   "archive failure error",
					})
				})

				Convey(`If remote report returns success, ACK.`, func() {
					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldBeNil)
					So(ack, ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project: project,
						Path:    archiveTask.Path,
						Error:   "archive failure error",
					})
				})

				Convey(`If an empty error string is supplied, the generic error will be filled in.`, func() {
					archiveErr = errors.New("")

					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldBeNil)
					So(ack, ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project: project,
						Path:    archiveTask.Path,
						Error:   "archival error",
					})
				})
			})
		})

		Convey(`When not enforcing stream completeness`, func() {
			stream.Age = google.NewDuration(expired)

			Convey(`With no terminal index`, func() {
				Convey(`Will successfully archive if there are no entries.`, func() {
					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldBeNil)
					So(ack, ShouldBeTrue)

					So(hasStreams(true, true, false), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project:       project,
						Path:          archiveTask.Path,
						LogEntryCount: 0,
						TerminalIndex: -1,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
						DataUrl:   gsURL(project, "data.bin"),
					})
				})

				Convey(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index.`, func() {
					addTestEntry(project, 0, 1, 2, 4)

					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldBeNil)
					So(ack, ShouldBeTrue)

					So(hasStreams(true, true, true), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project:       project,
						Path:          archiveTask.Path,
						LogEntryCount: 4,
						TerminalIndex: 4,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
						DataUrl:   gsURL(project, "data.bin"),
					})
				})
			})

			Convey(`With terminal index 3`, func() {
				stream.State.TerminalIndex = 3

				Convey(`Will successfully archive if there are no entries.`, func() {
					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldBeNil)
					So(ack, ShouldBeTrue)

					So(hasStreams(true, true, false), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project:       project,
						Path:          archiveTask.Path,
						LogEntryCount: 0,
						TerminalIndex: -1,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
						DataUrl:   gsURL(project, "data.bin"),
					})
				})

				Convey(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index to 2.`, func() {
					addTestEntry(project, 0, 1, 2, 4)

					ack, err := ar.archiveTaskImpl(c, task)
					So(err, ShouldBeNil)
					So(ack, ShouldBeTrue)

					So(hasStreams(true, true, true), ShouldBeTrue)
					So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
						Project:       project,
						Path:          archiveTask.Path,
						LogEntryCount: 3,
						TerminalIndex: 2,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
						DataUrl:   gsURL(project, "data.bin"),
					})
				})
			})
		})

		Convey(`With an empty project`, func() {
			archiveTask.Project = ""

			Convey(`Will successfully archive {0, 1, 2, 3} with terminal index 3 using "_" for project archive path.`, func() {
				stream.State.TerminalIndex = 3
				addTestEntry("", 0, 1, 2, 3)

				ack, err := ar.archiveTaskImpl(c, task)
				So(err, ShouldBeNil)
				So(ack, ShouldBeTrue)

				So(hasStreams(true, true, true), ShouldBeTrue)
				So(archiveRequest, ShouldResemble, &logdog.ArchiveStreamRequest{
					Project:       "",
					Path:          archiveTask.Path,
					LogEntryCount: 4,
					TerminalIndex: 3,

					StreamUrl: gsURL("_", "logstream.entries"),
					IndexUrl:  gsURL("_", "logstream.index"),
					DataUrl:   gsURL("_", "data.bin"),
				})
			})
		})

		// Simulate failures during the various stream generation operations.
		Convey(`Stream generation failures`, func() {
			stream.State.TerminalIndex = 3
			addTestEntry(project, 0, 1, 2, 3)

			for _, failName := range []string{"/logstream.entries", "/logstream.index", "/data.bin"} {
				for _, testCase := range []struct {
					name  string
					setup func()
				}{
					{"writer create failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								return errors.WrapTransient(errors.New("test error"))
							}
							return nil
						}
					}},

					{"write failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								w.writeErr = errors.WrapTransient(errors.New("test error"))
							}
							return nil
						}
					}},

					{"rename failure", func() {
						gsc.renameErr = func(src, dst gs.Path) error {
							if strings.HasSuffix(string(src), failName) {
								return errors.WrapTransient(errors.New("test error"))
							}
							return nil
						}
					}},

					{"close failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								w.closeErr = errors.WrapTransient(errors.New("test error"))
							}
							return nil
						}
					}},

					{"delete failure after other failure", func() {
						// Simulate a write failure. This is the error that will actually
						// be returned.
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								w.writeErr = errors.WrapTransient(errors.New("test error"))
							}
							return nil
						}

						// This will trigger whe NewWriter fails from the above
						// instrumentation.
						gsc.deleteErr = func(p gs.Path) error {
							if strings.HasSuffix(string(p), failName) {
								return errors.New("other error")
							}
							return nil
						}
					}},
				} {
					Convey(fmt.Sprintf(`Can handle %s for %s, and will not archive.`, testCase.name, failName), func() {
						testCase.setup()

						ack, err := ar.archiveTaskImpl(c, task)
						So(err, ShouldErrLike, "test error")
						So(ack, ShouldBeFalse)
						So(archiveRequest, ShouldBeNil)
					})
				}
			}
		})
	})
}
