// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package backend

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	ds "github.com/luci/gae/service/datastore"
	tq "github.com/luci/gae/service/taskqueue"
	"github.com/luci/luci-go/appengine/gaetesting"
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	ct "github.com/luci/luci-go/appengine/logdog/coordinator/coordinatorTest"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/gcloud/gs"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/proto/logdog/svcconfig"
	"github.com/luci/luci-go/server/logdog/storage"
	"github.com/luci/luci-go/server/logdog/storage/memory"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

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
		c := gaetesting.TestingContext()
		c, tc := testclock.UseTime(c, testclock.TestTimeUTC)

		st := memory.Storage{}
		gsc := testGSClient{}

		cfg := svcconfig.Coordinator{
			ArchiveGsBucket:         "archive-test",
			ArchiveGsBasePath:       "/path/to/archive/", // Extra slashes, test trim.
			StorageCleanupTaskQueue: "storage-cleanup-test-queue",

			ArchiveDelayMax: google.NewDuration(24 * time.Hour),
		}
		c = ct.UseConfig(c, &cfg)
		tq.Get(c).Testable().CreateQueue(cfg.StorageCleanupTaskQueue)

		b := Backend{
			s: coordinator.Service{
				StorageFunc: func(context.Context) (storage.Storage, error) {
					return &st, nil
				},
				GSClientFunc: func(context.Context) (gs.Client, error) {
					return &gsc, nil
				},
			},
		}

		do := func(path string) error {
			// Make a minimal request.
			req := http.Request{
				Form: url.Values{
					"path": []string{path},
				},
			}
			return b.handleArchiveTask(c, &req)
		}

		Convey(`Test HTTP handler`, func() {
			tb := testBase{Context: c}

			r := httprouter.New()
			b.InstallHandlers(r, tb.base)

			s := httptest.NewServer(r)
			defer s.Close()

			Convey(`Will return HTTP OK for if handleArchiveTask returns nil.`, func() {
				// No stream name = invalid stream.
				resp, err := http.PostForm(fmt.Sprintf("%s/archive/handle", s.URL), nil)
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusOK)
			})

			Convey(`Will return HTTP InternalStatusError if handleArchiveTask returns non-nil.`, func() {
				resp, err := http.PostForm(fmt.Sprintf("%s/archive/handle", s.URL), mkValues(map[string]string{
					"path": "testing/+/foo",
				}))
				So(err, ShouldBeNil)
				So(resp.StatusCode, ShouldEqual, http.StatusInternalServerError)
			})
		})

		Convey(`Will no-op archive successfully if an invalid path is specified.`, func() {
			So(do("!!!invalid!!!"), ShouldBeNil)
		})

		Convey(`Will fail to archive if the specified path does not exist.`, func() {
			So(do("testing/+/foo"), ShouldEqual, ds.ErrNoSuchEntity)
		})

		Convey(`Will fail to archive if no config is loaded.`, func() {
			c = ct.UseConfig(c, nil)
			So(do("testing/+/foo"), ShouldErrLike, "settings are not available")
		})

		Convey(`When archiving a log stream`, func() {
			lsDesc := ct.TestLogStreamDescriptor(c, "foo")
			ls, err := ct.TestLogStream(c, lsDesc)
			So(err, ShouldBeNil)

			// Utility function to add a log entry for "ls".
			addTestEntry := func(idxs ...int) {
				for _, v := range idxs {
					le := ct.TestLogEntry(c, ls, v)

					d, err := proto.Marshal(le)
					if err != nil {
						panic(err)
					}

					err = st.Put(&storage.PutRequest{
						Path:  types.StreamPath("testing/+/foo"),
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

			// Advance the time so we can check for modified state by testing if
			// Updated != Created.
			tc.Add(time.Second)

			Convey(`Will refrain from archiving if the stream is already archived.`, func() {
				ls.State = coordinator.LSArchived
				So(ls.Archived(), ShouldBeTrue)
				So(ls.Put(ds.Get(c)), ShouldBeNil)

				So(do("testing/+/foo"), ShouldBeNil)

				So(ds.Get(c).Get(ls), ShouldBeNil)
				So(ls.Updated, ShouldResembleV, ls.Created)
			})

			Convey(`When we're within the stream's maximum completeness delay`, func() {
				// Weird case: the log has been marked for archival, has not been
				// terminated, and is within its completeness delay. This task will not
				// have been dispatched by our archive cron, but let's assert that it
				// behaves correctly regardless.
				Convey(`When there is no terminal index`, func() {
					ls.TerminalIndex = -1
					So(ls.Put(ds.Get(c)), ShouldBeNil)

					Convey(`Will succeed if the log stream had no entries and no terminal index.`, func() {
						So(do("testing/+/foo"), ShouldBeNil)

						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.Updated, ShouldResembleV, ds.RoundTime(tc.Now().UTC()))
						So(ls.Archived(), ShouldBeTrue)
						So(ls.TerminalIndex, ShouldEqual, -1)
					})
				})

				Convey(`With terminal index "3"`, func() {
					ls.TerminalIndex = 3
					So(ls.Put(ds.Get(c)), ShouldBeNil)

					Convey(`Will fail if the log stream had a terminal index and no entries.`, func() {
						So(do("testing/+/foo"), ShouldErrLike, errArchiveFailed)
					})

					Convey(`Will fail to archive {0, 1, 2, 4} (incomplete).`, func() {
						addTestEntry(0, 1, 2, 4)

						So(do("testing/+/foo"), ShouldErrLike, "non-contiguous log stream")
					})

					Convey(`Will successfully archive {0, 1, 2, 3, 4}, stopping at the terminal index.`, func() {
						addTestEntry(0, 1, 2, 3, 4)

						So(do("testing/+/foo"), ShouldBeNil)

						So(ds.Get(c).Get(ls), ShouldBeNil)
						So(ls.State, ShouldEqual, coordinator.LSArchived)
						So(ls.Updated, ShouldResembleV, ds.RoundTime(tc.Now().UTC()))
						So(ls.TerminalIndex, ShouldEqual, 3)
						So(ls.ArchiveStreamURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/logstream.entries")
						So(ls.ArchiveStreamSize, ShouldBeGreaterThan, 0)
						So(ls.ArchiveIndexURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/logstream.index")
						So(ls.ArchiveIndexSize, ShouldBeGreaterThan, 0)
						So(ls.ArchiveDataURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/data.bin")
						So(ls.ArchiveDataSize, ShouldBeGreaterThan, 0)
					})
				})

				Convey(`When we're past the streams' maximum completeness delay`, func() {
					tc.Add(cfg.ArchiveDelayMax.Duration())

					Convey(`With no terminal index`, func() {
						ls.TerminalIndex = -1
						So(ls.Put(ds.Get(c)), ShouldBeNil)

						Convey(`Will successfully archive if there are no entries.`, func() {
							So(do("testing/+/foo"), ShouldBeNil)

							So(ds.Get(c).Get(ls), ShouldBeNil)
							So(ls.Updated, ShouldResembleV, ds.RoundTime(tc.Now().UTC()))
							So(ls.Archived(), ShouldBeTrue)
						})

						Convey(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index.`, func() {
							addTestEntry(0, 1, 2, 4)

							So(do("testing/+/foo"), ShouldBeNil)

							So(ds.Get(c).Get(ls), ShouldBeNil)
							So(ls.State, ShouldEqual, coordinator.LSArchived)
							So(ls.Updated, ShouldResembleV, ds.RoundTime(tc.Now().UTC()))
							So(ls.TerminalIndex, ShouldEqual, 4)
							So(ls.ArchiveStreamURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/logstream.entries")
							So(ls.ArchiveStreamSize, ShouldBeGreaterThan, 0)
							So(ls.ArchiveIndexURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/logstream.index")
							So(ls.ArchiveIndexSize, ShouldBeGreaterThan, 0)
							So(ls.ArchiveDataURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/data.bin")
							So(ls.ArchiveDataSize, ShouldBeGreaterThan, 0)
						})
					})

					Convey(`With terminal index 3`, func() {
						ls.TerminalIndex = 3
						So(ls.Put(ds.Get(c)), ShouldBeNil)

						Convey(`Will successfully archive if there are no entries.`, func() {

							So(do("testing/+/foo"), ShouldBeNil)

							So(ds.Get(c).Get(ls), ShouldBeNil)
							So(ls.Updated, ShouldResembleV, ds.RoundTime(tc.Now().UTC()))
							So(ls.Archived(), ShouldBeTrue)
						})

						Convey(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index.`, func() {
							addTestEntry(0, 1, 2, 4)

							So(do("testing/+/foo"), ShouldBeNil)

							So(ds.Get(c).Get(ls), ShouldBeNil)
							So(ls.State, ShouldEqual, coordinator.LSArchived)
							So(ls.Updated, ShouldResembleV, ds.RoundTime(tc.Now().UTC()))
							So(ls.TerminalIndex, ShouldEqual, 2)
							So(ls.ArchiveStreamURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/logstream.entries")
							So(ls.ArchiveStreamSize, ShouldBeGreaterThan, 0)
							So(ls.ArchiveIndexURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/logstream.index")
							So(ls.ArchiveIndexSize, ShouldBeGreaterThan, 0)
							So(ls.ArchiveDataURL, ShouldEqual, "gs://archive-test/path/to/archive/testing/+/foo/data.bin")
							So(ls.ArchiveDataSize, ShouldBeGreaterThan, 0)
						})
					})
				})
			})

			// Simulate failures during the various stream generation operations.
			Convey(`Stream generation failures`, func() {
				ls.TerminalIndex = 3
				So(ls.Put(ds.Get(c)), ShouldBeNil)
				addTestEntry(0, 1, 2, 3)

				for _, stream := range []string{"/logstream.entries", "/logstream.index", "/data.bin"} {
					stream := stream

					for _, testCase := range []struct {
						name  string
						setup func()
					}{
						{"delete failure", func() {
							gsc.deleteErr = func(b, p string) error {
								if strings.HasSuffix(p, stream) {
									return errors.New("test error")
								}
								return nil
							}
						}},

						{"writer create failure", func() {
							gsc.newWriterErr = func(w *testGSWriter) error {
								if strings.HasSuffix(w.path, stream) {
									return errors.New("test error")
								}
								return nil
							}
						}},

						{"write failure", func() {
							gsc.newWriterErr = func(w *testGSWriter) error {
								if strings.HasSuffix(w.path, stream) {
									w.writeErr = errors.New("test error")
								}
								return nil
							}
						}},

						{"close failure", func() {
							gsc.newWriterErr = func(w *testGSWriter) error {
								if strings.HasSuffix(w.path, stream) {
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
								if strings.HasSuffix(w.path, stream) {
									w.writeErr = errors.New("test error")
								}
								return nil
							}

							// This will trigger twice per stream, once on create and once on
							// cleanup after the write fails. We want to return an error in
							// the latter case.
							gsc.deleteErr = func(b, p string) error {
								if strings.HasSuffix(p, stream) {
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
						Convey(fmt.Sprintf(`Can handle %s for %s`, testCase.name, stream), func() {
							So(ls.Put(ds.Get(c)), ShouldBeNil)
							testCase.setup()

							So(do("testing/+/foo"), ShouldErrLike, "test error")

							So(ds.Get(c).Get(ls), ShouldBeNil)
							So(ls.Updated, ShouldResembleV, ls.Created)
							So(ls.Archived(), ShouldBeFalse)
						})
					}
				}
			})
		})
	})
}
