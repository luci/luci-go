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

package archivist

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	gaemem "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/storage"
	"go.chromium.org/luci/logdog/common/storage/memory"
	"go.chromium.org/luci/logdog/common/types"
	srvcfg "go.chromium.org/luci/logdog/server/config"

	"google.golang.org/protobuf/proto"

	cl "cloud.google.com/go/logging"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
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
	*emptypb.Empty, error) {
	if cb := sc.asCallback; cb != nil {
		if err := cb(req); err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
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
	if l := len(p.Filename()); l > 1024 {
		panic(fmt.Errorf("too long filepath %d: %q", l, p))
	}
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

// testCLClient iis a testing implementation of the CLClient interface.
type testCLClient struct {
	closeFn  func() error
	pingFn   func(context.Context) error
	loggerFn func(string, ...cl.LoggerOption) *cl.Logger

	isClosed    bool
	clProject   string
	luciProject string
}

func (c *testCLClient) Close() error {
	if c.isClosed {
		panic("double close")
	}
	c.isClosed = true
	if c.closeFn != nil {
		return c.closeFn()
	}
	return nil
}

func (c *testCLClient) Ping(ctx context.Context) error {
	if c.pingFn != nil {
		return c.pingFn(ctx)
	}
	return nil
}

func (c *testCLClient) Logger(logID string, opts ...cl.LoggerOption) *cl.Logger {
	if c.loggerFn != nil {
		return c.loggerFn(logID, opts...)
	}
	return nil
}

func TestHandleArchive(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing archive setup`, t, func(t *ftt.Test) {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeUTC)
		c, _ = tsmon.WithDummyInMemory(c)
		ms := tsmon.Store(c)
		c = gaemem.Use(c)
		c = srvcfg.WithStore(c, &srvcfg.Store{})

		st := memory.Storage{}
		gsc := testGSClient{}
		gscFactory := func(context.Context, string) (gs.Client, error) {
			return &gsc, nil
		}

		var clc *testCLClient
		clcFactory := func(ctx context.Context, luciProject, clProject string, onError func(err error)) (CLClient, error) {
			clc = &testCLClient{
				clProject:   clProject,
				luciProject: luciProject,
			}
			return clc, nil
		}
		// Set up our test log stream.
		project := "test-project"
		clProject := "test-cloud-project"

		desc := logpb.LogStreamDescriptor{
			Prefix: "testing",
			Name:   "foo",
		}

		// mock project config
		lucicfg := map[config.Set]cfgmem.Files{
			"services/${appid}": {
				"services.cfg": `coordinator { admin_auth_group: "a" }`,
			},
			config.Set("projects/" + project): {
				"${appid}.cfg": `archive_gs_bucket: "a"`,
			},
		}
		c = cfgclient.Use(c, cfgmem.New(lucicfg))
		assert.Loosely(t, srvcfg.Sync(c), should.BeNil)
		datastore.GetTestable(c).CatchupIndexes()

		// Utility function to add a log entry for "ls".
		addTestEntry := func(p string, idxs ...int) {
			for _, v := range idxs {
				le := logpb.LogEntry{
					PrefixIndex: uint64(v),
					StreamIndex: uint64(v),
					Content: &logpb.LogEntry_Text{&logpb.Text{
						Lines: []*logpb.Text_Line{
							{
								Value:     []byte(fmt.Sprintf("line #%d", v)),
								Delimiter: "\n",
							},
						},
					}},
				}

				d, err := proto.Marshal(&le)
				if err != nil {
					panic(err)
				}

				err = st.Put(c, storage.PutRequest{
					Project: p,
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
		task := &logdog.ArchiveTask{
			Project: project,
			Id:      "coordinator-stream-id",
			Realm:   "foo:bar",
		}
		expired++ // This represents a time PAST CompletePeriod.

		// Set up our test Coordinator client stubs.
		stream := logdog.LoadStreamResponse{
			State: &logdog.InternalLogStreamState{
				ProtoVersion:  logpb.Version,
				TerminalIndex: -1,
				Archived:      false,
				Purged:        false,
			},
		}

		// Allow tests to modify the log stream descriptor.
		reloadDesc := func() {
			descBytes, err := proto.Marshal(&desc)
			if err != nil {
				panic(err)
			}
			stream.Desc = descBytes
		}
		reloadDesc()

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

		stBase := Settings{}
		CLBufferLimit := 12334
		ar := Archivist{
			Service: &sc,
			SettingsLoader: func(c context.Context, project string) (*Settings, error) {
				// Extra slashes to test concatenation,.
				st := stBase
				st.GSBase = gs.Path(fmt.Sprintf("gs://archival/%s/path/to/archive/", project))
				st.GSStagingBase = gs.Path(fmt.Sprintf("gs://archival-staging/%s/path/to/archive/", project))
				st.CloudLoggingProjectID = func() string { return clProject }()
				st.CloudLoggingBufferLimit = CLBufferLimit
				return &st, nil
			},
			Storage:         &st,
			GSClientFactory: gscFactory,
			CLClientFactory: clcFactory,
		}

		gsURL := func(project, name string) string {
			return fmt.Sprintf("gs://archival/%s/path/to/archive/%s/%s/%s", project, project, desc.Path(), name)
		}

		// hasStreams can be called to check that the retained archiveRequest had
		// data sizes for the named archive stream types.
		//
		// After checking, the values are set to zero. This allows us to use
		// ShouldEqual without hardcoding specific archival sizes into the results.
		hasStreams := func(log, index, data bool) bool {
			assert.Loosely(t, archiveRequest, should.NotBeNil)
			if (log && archiveRequest.StreamSize <= 0) ||
				(index && archiveRequest.IndexSize <= 0) {
				return false
			}

			archiveRequest.StreamSize = 0
			archiveRequest.IndexSize = 0
			return true
		}

		t.Run(`Will return task and fail to archive if the specified stream state could not be loaded.`, func(t *ftt.Test) {
			sc.lsCallback = func(*logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
				return nil, errors.New("does not exist")
			}

			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.ErrLike("does not exist"))
		})

		t.Run(`Will consume task and refrain from archiving if the stream is already archived.`, func(t *ftt.Test) {
			stream.State.Archived = true

			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
			assert.Loosely(t, archiveRequest, should.BeNil)
		})

		t.Run(`Will consume task and refrain from archiving if the stream is purged.`, func(t *ftt.Test) {
			stream.State.Purged = true

			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
			assert.Loosely(t, archiveRequest, should.BeNil)
		})

		t.Run(`With terminal index "3"`, func(t *ftt.Test) {
			stream.State.TerminalIndex = 3

			t.Run(`Will consume the task if the log stream has no entries.`, func(t *ftt.Test) {
				assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)
			})

			t.Run(`Will archive {0, 1, 2, 4} (incomplete).`, func(t *ftt.Test) {
				addTestEntry(project, 0, 1, 2, 4)
				assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(4))
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)
			})

			t.Run(`Will successfully archive {0, 1, 2, 3, 4}, stopping at the terminal index.`, func(t *ftt.Test) {
				addTestEntry(project, 0, 1, 2, 3, 4)

				assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(5))
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)

				assert.Loosely(t, hasStreams(true, true, true), should.BeTrue)

				assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
					Project:       project,
					Id:            task.Id,
					LogEntryCount: 4,
					TerminalIndex: 3,

					StreamUrl: gsURL(project, "logstream.entries"),
					IndexUrl:  gsURL(project, "logstream.index"),
				}))
			})

			t.Run(`Will truncate long descriptor paths in GS filenames`, func(t *ftt.Test) {
				desc.Name = strings.Repeat("very/long/prefix/", 200)
				assert.Loosely(t, len(desc.Path()), should.BeGreaterThan(2048))
				reloadDesc()
				addTestEntry(project, 0, 1)

				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)

				// GS allows up to 1024 bytes long names.
				assert.Loosely(t, len(archiveRequest.StreamUrl[len("gs://archival/"):]), should.BeLessThan(1024))
				assert.Loosely(t, len(archiveRequest.IndexUrl[len("gs://archival/"):]), should.BeLessThan(1024))

				// While not essential, it's nice to put both files under the same
				// GS "directory".
				gsDirName := func(p string) string { return p[0:strings.LastIndex(p, "/")] }
				assert.Loosely(t, gsDirName(archiveRequest.StreamUrl), should.Equal(gsDirName(archiveRequest.IndexUrl)))
			})

			t.Run(`When a transient archival error occurs, will not consume the task.`, func(t *ftt.Test) {
				addTestEntry(project, 0, 1, 2, 3, 4)
				gsc.newWriterErr = func(*testGSWriter) error { return errors.New("test error", transient.Tag) }

				assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(5))
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.ErrLike("test error"))
				assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(5))
			})

			t.Run(`When a non-transient archival error occurs`, func(t *ftt.Test) {
				addTestEntry(project, 0, 1, 2, 3, 4)
				archiveErr := errors.New("archive failure error")
				gsc.newWriterErr = func(*testGSWriter) error { return archiveErr }

				t.Run(`If remote report returns an error, do not consume the task.`, func(t *ftt.Test) {
					archiveStreamErr = errors.New("test error", transient.Tag)

					assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(5))
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.ErrLike("test error"))
					assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(5))

					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project: project,
						Id:      task.Id,
						Error:   "archive failure error",
					}))
				})

				t.Run(`If remote report returns success, the task is consumed.`, func(t *ftt.Test) {
					assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(5))
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)
					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project: project,
						Id:      task.Id,
						Error:   "archive failure error",
					}))
				})

				t.Run(`If an empty error string is supplied, the generic error will be filled in.`, func(t *ftt.Test) {
					archiveErr = errors.New("")

					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project: project,
						Id:      task.Id,
						Error:   "archival error",
					}))
				})
			})
		})

		t.Run(`When not enforcing stream completeness`, func(t *ftt.Test) {
			stream.Age = durationpb.New(expired)

			t.Run(`With no terminal index`, func(t *ftt.Test) {
				t.Run(`Will successfully archive if there are no entries.`, func(t *ftt.Test) {
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)

					assert.Loosely(t, hasStreams(true, true, false), should.BeTrue)
					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project:       project,
						Id:            task.Id,
						LogEntryCount: 0,
						TerminalIndex: -1,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
					}))
				})

				t.Run(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index.`, func(t *ftt.Test) {
					addTestEntry(project, 0, 1, 2, 4)

					assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(4))
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)

					assert.Loosely(t, hasStreams(true, true, true), should.BeTrue)
					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project:       project,
						Id:            task.Id,
						LogEntryCount: 4,
						TerminalIndex: 4,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
					}))
				})
			})

			t.Run(`With terminal index 3`, func(t *ftt.Test) {
				stream.State.TerminalIndex = 3

				t.Run(`Will successfully archive if there are no entries.`, func(t *ftt.Test) {
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)

					assert.Loosely(t, hasStreams(true, true, false), should.BeTrue)
					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project:       project,
						Id:            task.Id,
						LogEntryCount: 0,
						TerminalIndex: -1,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
					}))
				})

				t.Run(`With {0, 1, 2, 4} (incomplete) will archive the stream and update its terminal index to 2.`, func(t *ftt.Test) {
					addTestEntry(project, 0, 1, 2, 4)

					assert.Loosely(t, st.Count(project, desc.Path()), should.Equal(4))
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, st.Count(project, desc.Path()), should.BeZero)

					assert.Loosely(t, hasStreams(true, true, true), should.BeTrue)
					assert.Loosely(t, archiveRequest, should.Resemble(&logdog.ArchiveStreamRequest{
						Project:       project,
						Id:            task.Id,
						LogEntryCount: 3,
						TerminalIndex: 2,

						StreamUrl: gsURL(project, "logstream.entries"),
						IndexUrl:  gsURL(project, "logstream.index"),
					}))
				})
			})
		})

		t.Run(`With an empty project name, will fail and consume the task.`, func(t *ftt.Test) {
			task.Project = ""

			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
		})

		t.Run(`With a project name, of which config doesn't exist, will fail and consume the task`, func(t *ftt.Test) {
			task.Project = "valid-project-but-cfg-not-exist"

			_, ok := lucicfg[config.Set("projects/"+task.Project)]
			assert.Loosely(t, ok, should.BeFalse)
			assert.Loosely(t, config.ValidateProjectName(task.Project), should.BeNil)
			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
		})

		t.Run(`With an invalid project name, will fail and consume the task.`, func(t *ftt.Test) {
			task.Project = "!!! invalid project name !!!"

			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
		})

		// Simulate failures during the various stream generation operations.
		t.Run(`Stream generation failures`, func(t *ftt.Test) {
			stream.State.TerminalIndex = 3
			addTestEntry(project, 0, 1, 2, 3)

			for _, failName := range []string{"/logstream.entries", "/logstream.index"} {
				for _, testCase := range []struct {
					name  string
					setup func()
				}{
					{"writer create failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								return errors.New("test error", transient.Tag)
							}
							return nil
						}
					}},

					{"write failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								w.writeErr = errors.New("test error", transient.Tag)
							}
							return nil
						}
					}},

					{"rename failure", func() {
						gsc.renameErr = func(src, dst gs.Path) error {
							if strings.HasSuffix(string(src), failName) {
								return errors.New("test error", transient.Tag)
							}
							return nil
						}
					}},

					{"close failure", func() {
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								w.closeErr = errors.New("test error", transient.Tag)
							}
							return nil
						}
					}},

					{"delete failure after other failure", func() {
						// Simulate a write failure. This is the error that will actually
						// be returned.
						gsc.newWriterErr = func(w *testGSWriter) error {
							if strings.HasSuffix(string(w.path), failName) {
								w.writeErr = errors.New("test error", transient.Tag)
							}
							return nil
						}

						// This will trigger when NewWriter fails from the above
						// instrumentation.
						gsc.deleteErr = func(p gs.Path) error {
							if strings.HasSuffix(string(p), failName) {
								return errors.New("other error")
							}
							return nil
						}
					}},
				} {
					t.Run(fmt.Sprintf(`Can handle %s for %s, and will not archive.`, testCase.name, failName), func(t *ftt.Test) {
						testCase.setup()

						assert.Loosely(t, ar.archiveTaskImpl(c, task), should.ErrLike("test error"))
						assert.Loosely(t, archiveRequest, should.BeNil)
					})
				}
			}
		})

		t.Run(`Will update metric`, func(t *ftt.Test) {
			fv := func(vs ...any) []any {
				ret := []any{project}
				return append(ret, vs...)
			}
			dSum := func(val any) float64 {
				return val.(*distribution.Distribution).Sum()
			}

			t.Run(`tsCount`, func(t *ftt.Test) {
				t.Run(`With failure`, func(t *ftt.Test) {
					sc.lsCallback = func(*logdog.LoadStreamRequest) (*logdog.LoadStreamResponse, error) {
						return nil, errors.New("beep")
					}
					assert.Loosely(t, ar.ArchiveTask(c, task), should.ErrLike("beep"))
					assert.Loosely(t, ms.Get(c, tsCount, fv(false)), should.Equal(1))
				})
				t.Run(`With success`, func(t *ftt.Test) {
					assert.Loosely(t, ar.ArchiveTask(c, task), should.BeNil)
					assert.Loosely(t, ms.Get(c, tsCount, fv(true)), should.Equal(1))
				})
			})

			t.Run(`All others`, func(t *ftt.Test) {
				addTestEntry(project, 0, 1, 2, 3, 4)
				assert.Loosely(t, ar.ArchiveTask(c, task), should.BeNil)

				st := logpb.StreamType_TEXT.String()
				assert.That(t, dSum(ms.Get(c, tsSize, fv("entries", st))), should.Equal(116.))
				assert.That(t, dSum(ms.Get(c, tsSize, fv("index", st))), should.Equal(58.))
				assert.Loosely(t, ms.Get(c, tsTotalBytes, fv("entries", st)), should.Equal(116))
				assert.Loosely(t, ms.Get(c, tsTotalBytes, fv("index", st)), should.Equal(58))
				assert.That(t, dSum(ms.Get(c, tsLogEntries, fv(st))), should.Equal(5.))
				assert.Loosely(t, ms.Get(c, tsTotalLogEntries, fv(st)), should.Equal(5))
			})
		})

		t.Run(`Will construct CLClient if CloudLoggingProjectID is set.`, func(t *ftt.Test) {
			desc.StreamType = logpb.StreamType_TEXT
			reloadDesc()

			t.Run(`w/ projectScope`, func(t *ftt.Test) {
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, clc, should.NotBeNil)
				assert.Loosely(t, clc.clProject, should.Equal(clProject))
				assert.Loosely(t, clc.luciProject, should.Equal(project))
			})

			t.Run(`w/ CommonLabels`, func(t *ftt.Test) {
				var opts []cl.LoggerOption
				ar.CLClientFactory = func(ctx context.Context, lp, cp string, onError func(err error)) (CLClient, error) {
					clc, err := clcFactory(c, lp, cp, func(err error) {})
					clc.(*testCLClient).loggerFn = func(n string, los ...cl.LoggerOption) *cl.Logger {
						opts = los
						return &cl.Logger{}
					}
					return clc, err
				}

				// CommonLabels returns a private type, commonLabels.
				findCommonLabels := func(opts []cl.LoggerOption) cl.LoggerOption {
					labelsType := reflect.TypeOf(cl.CommonLabels(nil))
					for _, opt := range opts {
						if reflect.TypeOf(opt) == labelsType {
							return opt
						}
					}
					return nil
				}

				t.Run(`w/ realm`, func(t *ftt.Test) {
					task.Realm = "project:bucket"
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, clc, should.NotBeNil)
					assert.Loosely(t, findCommonLabels(opts), should.Resemble(cl.CommonLabels(
						map[string]string{"realm": "project:bucket"})))
				})

				t.Run(`w/o realm`, func(t *ftt.Test) {
					task.Realm = ""
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, clc, should.NotBeNil)
					assert.Loosely(t, findCommonLabels(opts), should.Resemble(cl.CommonLabels(
						map[string]string{})))
				})
			})
		})

		t.Run(`Will not construct CLClient`, func(t *ftt.Test) {
			t.Run(`if CloudLoggingProjectID is not set.`, func(t *ftt.Test) {
				desc.StreamType = logpb.StreamType_TEXT
				reloadDesc()

				clProject = ""
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, clc, should.BeNil)
			})

			t.Run(`if StreamType is not TEXT.`, func(t *ftt.Test) {
				desc.StreamType = logpb.StreamType_BINARY
				reloadDesc()

				clProject = ""
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, clc, should.BeNil)
			})
		})

		t.Run(`Will close CLClient`, func(t *ftt.Test) {
			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
			assert.Loosely(t, clc.isClosed, should.BeTrue)
		})

		t.Run("Will validate CloudLoggingProjectID.", func(t *ftt.Test) {
			clProject = "123-foo"
			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.ErrLike("must start with a lowercase"))
		})

		t.Run("Will ping", func(t *ftt.Test) {
			ar.CLClientFactory = func(ctx context.Context, lp, cp string, onError func(err error)) (CLClient, error) {
				clc, err := clcFactory(c, lp, cp, func(err error) {})
				clc.(*testCLClient).pingFn = func(context.Context) error {
					return errors.New("Permission Denied")
				}
				return clc, err
			}
			assert.Loosely(t, ar.archiveTaskImpl(c, task), should.ErrLike("failed to ping"))
		})

		t.Run(`Will construct Cloud Logger`, func(t *ftt.Test) {
			// override the loggerFn to hook the params for the logger constructor.
			var logID string
			var opts []cl.LoggerOption
			ar.CLClientFactory = func(ctx context.Context, lp, cp string, onError func(err error)) (CLClient, error) {
				clc, err := clcFactory(c, lp, cp, func(err error) {})
				clc.(*testCLClient).loggerFn = func(l string, os ...cl.LoggerOption) *cl.Logger {
					logID, opts = l, os
					return &cl.Logger{}
				}
				return clc, err
			}

			t.Run("With BufferedByteLimit", func(t *ftt.Test) {
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
				assert.Loosely(t, logID, should.Equal("luci-logs"))
				assert.Loosely(t, opts[2], should.Resemble(cl.BufferedByteLimit(CLBufferLimit*1024*1024)))
			})

			t.Run("With MonitoredResource and labels", func(t *ftt.Test) {
				desc.Tags = map[string]string{"key1": "val1"}
				reloadDesc()
				assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)

				assert.Loosely(t, logID, should.Equal("luci-logs"))
				assert.Loosely(t, opts[0], should.Resemble(cl.CommonLabels(
					map[string]string{"key1": "val1", "realm": "foo:bar"},
				)))
				assert.Loosely(t, opts[1], should.Resemble(cl.CommonResource(&mrpb.MonitoredResource{
					Type: "generic_task",
					Labels: map[string]string{
						"project_id": project,
						"location":   desc.Name,
						"namespace":  desc.Prefix,
						"job":        "cloud-logging-export",
					},
				})))
			})

			t.Run("With luci.CloudLogExportID", func(t *ftt.Test) {
				t.Run("Valid", func(t *ftt.Test) {
					desc.Tags = map[string]string{"luci.CloudLogExportID": "try:pixel_1"}
					reloadDesc()
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, logID, should.Equal("try:pixel_1"))
				})

				t.Run("Empty", func(t *ftt.Test) {
					desc.Tags = map[string]string{"luci.CloudLogExportID": ""}
					reloadDesc()
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, logID, should.Equal("luci-logs"))
				})

				t.Run("Invalid chars", func(t *ftt.Test) {
					desc.Tags = map[string]string{"luci.CloudLogExportID": "/try:pixel_1"}
					reloadDesc()
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, logID, should.Equal("luci-logs"))
				})

				t.Run("Too long", func(t *ftt.Test) {
					longID := make([]rune, 512)
					for i := 0; i < 512; i++ {
						longID[i] = '1'
					}

					desc.Tags = map[string]string{"luci.CloudLogExportID": string(longID)}
					reloadDesc()
					assert.Loosely(t, ar.archiveTaskImpl(c, task), should.BeNil)
					assert.Loosely(t, logID, should.Equal("luci-logs"))
				})
			})
		})
	})
}

func TestStagingPaths(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		sa := stagedArchival{
			Settings: &Settings{
				GSBase:        gs.MakePath("base-bucket", "base-dir"),
				GSStagingBase: gs.MakePath("staging-bucket", "staging-dir"),
			},
			project: "some-project",
		}

		t.Run("Fits limits", func(t *ftt.Test) {
			sa.path = "some-prefix/+/a/b/c/d/e"
			assert.Loosely(t, sa.makeStagingPaths(120), should.BeNil)

			assert.Loosely(t, sa.stream.staged, should.Equal(gs.Path("gs://staging-bucket/staging-dir/some-project/p/lvAr3dzO3sXWufWt_4VTeV3-Me1qanKMnwLP90BacPQ/+/a/b/c/d/e/logstream.entries")))
			assert.Loosely(t, sa.stream.final, should.Equal(gs.Path("gs://base-bucket/base-dir/some-project/some-prefix/+/a/b/c/d/e/logstream.entries")))

			assert.Loosely(t, sa.index.staged, should.Equal(gs.Path("gs://staging-bucket/staging-dir/some-project/p/lvAr3dzO3sXWufWt_4VTeV3-Me1qanKMnwLP90BacPQ/+/a/b/c/d/e/logstream.index")))
			assert.Loosely(t, sa.index.final, should.Equal(gs.Path("gs://base-bucket/base-dir/some-project/some-prefix/+/a/b/c/d/e/logstream.index")))
		})

		t.Run("Gets truncated", func(t *ftt.Test) {
			sa.path = "some-prefix/+/1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
			assert.Loosely(t, sa.makeStagingPaths(120), should.BeNil)

			assert.Loosely(t, sa.stream.staged, should.Equal(gs.Path("gs://staging-bucket/staging-dir/some-project/p/lvAr3dzO3sXWufWt_4VTeV3-Me1qanKMnwLP90BacPQ/+/12-TRUNCATED-NatSoX9rqDX5JD2f/logstream.entries")))
			assert.Loosely(t, sa.stream.final, should.Equal(gs.Path("gs://base-bucket/base-dir/some-project/some-prefix/+/12-TRUNCATED-NatSoX9rqDX5JD2f/logstream.entries")))

			assert.Loosely(t, sa.index.staged, should.Equal(gs.Path("gs://staging-bucket/staging-dir/some-project/p/lvAr3dzO3sXWufWt_4VTeV3-Me1qanKMnwLP90BacPQ/+/12-TRUNCATED-NatSoX9rqDX5JD2f/logstream.index")))
			assert.Loosely(t, sa.index.final, should.Equal(gs.Path("gs://base-bucket/base-dir/some-project/some-prefix/+/12-TRUNCATED-NatSoX9rqDX5JD2f/logstream.index")))

			for _, p := range []stagingPaths{sa.stream, sa.index} {
				assert.Loosely(t, len(p.staged.Filename()), should.BeLessThanOrEqual(120))
				assert.Loosely(t, len(p.final.Filename()), should.BeLessThanOrEqual(120))
			}
		})
	})
}
