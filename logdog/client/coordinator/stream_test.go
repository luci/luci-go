// Copyright 2015 The LUCI Authors.
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

package coordinator

import (
	"errors"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"

	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func genLog(idx int64, id string) *logpb.LogEntry {
	return &logpb.LogEntry{
		StreamIndex: uint64(idx),
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{Value: id},
				},
			},
		},
	}
}

func genDG(idx int64, content ...string) []*logpb.LogEntry {
	var contentSize uint64
	if len(content) > 1 {
		for _, c := range content {
			contentSize += uint64(len(c))
		}
	}

	logs := make([]*logpb.LogEntry, len(content))
	for i, c := range content {
		dg := logpb.Datagram{
			Data: []byte(c),
		}
		if len(content) > 1 {
			dg.Partial = &logpb.Datagram_Partial{
				Index: uint32(i),
				Size:  contentSize,
				Last:  (i == len(content)-1),
			}
		}

		logs[i] = &logpb.LogEntry{
			StreamIndex: uint64(idx + int64(i)),
			Content:     &logpb.LogEntry_Datagram{&dg},
		}
	}
	return logs
}

// testStreamLogsService implements just the Get and Tail endpoints,
// instrumented for testing.
type testStreamLogsService struct {
	testLogsServiceBase

	// Get
	GR logdog.GetRequest
	GH func(*logdog.GetRequest) (*logdog.GetResponse, error)

	// Tail
	TR logdog.TailRequest
	TH func(*logdog.TailRequest) (*logdog.GetResponse, error)
}

func (s *testStreamLogsService) Get(c context.Context, req *logdog.GetRequest) (*logdog.GetResponse, error) {
	s.GR = *req
	if h := s.GH; h != nil {
		return s.GH(req)
	}
	return nil, errors.New("not implemented")
}

func (s *testStreamLogsService) Tail(c context.Context, req *logdog.TailRequest) (*logdog.GetResponse, error) {
	s.TR = *req
	if h := s.TH; h != nil {
		return s.TH(req)
	}
	return nil, errors.New("not implemented")
}

func TestStreamGet(t *testing.T) {
	t.Parallel()

	Convey(`A testing Client`, t, func() {
		c := context.Background()
		now := testclock.TestTimeUTC

		ts := prpctest.Server{}
		svc := testStreamLogsService{}
		logdog.RegisterLogsServer(&ts, &svc)

		// Create a testing server and client.
		ts.Start(c)
		defer ts.Close()

		prpcClient, err := ts.NewClient()
		if err != nil {
			panic(err)
		}
		client := Client{
			C: logdog.NewLogsPRPCClient(prpcClient),
		}

		Convey(`Can bind a Stream`, func() {
			s := client.Stream("myproj", "test/+/a")

			Convey(`Test Get`, func() {
				Convey(`A default Get query will return logs and no state.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{
								genLog(1337, "ohai"),
								genLog(1338, "kthxbye"),
							},
						}, nil
					}

					l, err := s.Get(c)
					So(err, ShouldBeNil)
					So(l, ShouldResemble, []*logpb.LogEntry{genLog(1337, "ohai"), genLog(1338, "kthxbye")})

					// Validate the correct parameters were sent.
					So(svc.GR, ShouldResemble, logdog.GetRequest{
						Project: "myproj",
						Path:    "test/+/a",
					})
				})

				Convey(`Will form a proper Get logs query.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{}, nil
					}

					l, err := s.Get(c, NonContiguous(), Index(1))
					So(err, ShouldBeNil)
					So(l, ShouldBeNil)

					// Validate the correct parameters were sent.
					So(svc.GR, ShouldResemble, logdog.GetRequest{
						Project:       "myproj",
						Path:          "test/+/a",
						NonContiguous: true,
						Index:         1,
					})
				})

				Convey(`Will request a specific number of logs if a constraint is supplied.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{
								genLog(1337, "ohai"),
							},
						}, nil
					}

					l, err := s.Get(c, LimitCount(64), LimitBytes(32))
					So(err, ShouldBeNil)
					So(l, ShouldResemble, []*logpb.LogEntry{genLog(1337, "ohai")})

					// Validate the HTTP request that we made.
					So(svc.GR, ShouldResemble, logdog.GetRequest{
						Project:   "myproj",
						Path:      "test/+/a",
						LogCount:  64,
						ByteCount: 32,
					})
				})

				Convey(`Can decode a full protobuf and state.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{
								genLog(1337, "kthxbye"),
							},
							State: &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
								Archive: &logdog.LogStreamState_ArchiveInfo{
									IndexUrl:  "index",
									StreamUrl: "stream",
									DataUrl:   "data",
								},
							},
							Desc: &logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.StreamType_TEXT,
							},
						}, nil
					}

					var ls LogStream
					l, err := s.Get(c, WithState(&ls))
					So(err, ShouldBeNil)
					So(l, ShouldResemble, []*logpb.LogEntry{genLog(1337, "kthxbye")})
					So(ls, ShouldResemble, LogStream{
						Path: "test/+/a",
						Desc: logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.StreamType_TEXT,
						},
						State: StreamState{
							Created:          now,
							Archived:         true,
							ArchiveIndexURL:  "index",
							ArchiveStreamURL: "stream",
							ArchiveDataURL:   "data",
						},
					})
				})

				Convey(`Will return ErrNoSuchStream if the stream is not found.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.NotFound
					}

					_, err := s.Get(c)
					So(err, ShouldEqual, ErrNoSuchStream)
				})

				Convey(`Will return ErrNoAccess if unauthenticated.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.Unauthenticated
					}

					_, err := s.Get(c)
					So(err, ShouldEqual, ErrNoAccess)
				})

				Convey(`Will return ErrNoAccess if permission is denied.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.PermissionDenied
					}

					_, err := s.Get(c)
					So(err, ShouldEqual, ErrNoAccess)
				})
			})

			Convey(`Test State`, func() {
				Convey(`Will request just the state if asked.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Project: "myproj",
							Desc: &logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.StreamType_TEXT,
							},
							State: &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
							},
						}, nil
					}

					l, err := s.State(c)
					So(err, ShouldBeNil)
					So(l, ShouldResemble, &LogStream{
						Project: "myproj",
						Path:    "test/+/a",
						Desc: logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.StreamType_TEXT,
						},
						State: StreamState{
							Created: now.UTC(),
						},
					})

					// Validate the HTTP request that we made.
					So(svc.GR, ShouldResemble, logdog.GetRequest{
						Project:  "myproj",
						Path:     "test/+/a",
						LogCount: -1,
						State:    true,
					})
				})

				Convey(`Will return ErrNoSuchStream if the stream is not found.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.NotFound
					}

					_, err := s.State(c)
					So(err, ShouldEqual, ErrNoSuchStream)
				})

				Convey(`Will return ErrNoAccess if unauthenticated.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.Unauthenticated
					}

					_, err := s.State(c)
					So(err, ShouldEqual, ErrNoAccess)
				})

				Convey(`Will return ErrNoAccess if permission is denied.`, func() {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.PermissionDenied
					}

					_, err := s.State(c)
					So(err, ShouldEqual, ErrNoAccess)
				})
			})

			Convey(`Test Tail`, func() {
				Convey(`Will form a proper Tail query.`, func() {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Project: "myproj",
							State: &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
							},
							Desc: &logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.StreamType_TEXT,
							},
							Logs: []*logpb.LogEntry{
								genLog(1337, "kthxbye"),
							},
						}, nil
					}

					var ls LogStream
					l, err := s.Tail(c, WithState(&ls))
					So(err, ShouldBeNil)

					// Validate the HTTP request that we made.
					So(svc.TR, ShouldResemble, logdog.TailRequest{
						Project: "myproj",
						Path:    "test/+/a",
						State:   true,
					})

					// Validate that the log and state were returned.
					So(l, ShouldResemble, genLog(1337, "kthxbye"))
					So(ls, ShouldResemble, LogStream{
						Project: "myproj",
						Path:    "test/+/a",
						Desc: logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.StreamType_TEXT,
						},
						State: StreamState{
							Created: now,
						},
					})
				})

				Convey(`Will return nil with state if no logs are returned from the endpoint.`, func() {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Project: "myproj",
							State: &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
							},
							Desc: &logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.StreamType_TEXT,
							},
						}, nil
					}

					var ls LogStream
					l, err := s.Tail(c, WithState(&ls))
					So(err, ShouldBeNil)
					So(l, ShouldBeNil)
					So(ls, ShouldResemble, LogStream{
						Project: "myproj",
						Path:    "test/+/a",
						Desc: logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.StreamType_TEXT,
						},
						State: StreamState{
							Created: now,
						},
					})
				})

				Convey(`Will error if multiple logs are returned from the endpoint.`, func() {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							State: &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
							},
							Logs: []*logpb.LogEntry{
								genLog(1337, "ohai"),
								genLog(1338, "kthxbye"),
							},
						}, nil
					}

					_, err := s.Tail(c)
					So(err, ShouldErrLike, "tail call returned 2 logs")
				})

				Convey(`Will return ErrNoSuchStream if the stream is not found.`, func() {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.NotFound
					}

					_, err := s.Tail(c)
					So(err, ShouldEqual, ErrNoSuchStream)
				})

				Convey(`Will return ErrNoAccess if unauthenticated.`, func() {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.Unauthenticated
					}

					_, err := s.Tail(c)
					So(err, ShouldEqual, ErrNoAccess)
				})

				Convey(`Will return ErrNoAccess if permission is denied.`, func() {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return nil, grpcutil.PermissionDenied
					}

					_, err := s.Tail(c)
					So(err, ShouldEqual, ErrNoAccess)
				})

				Convey(`When requesting complete streams`, func() {
					var allLogs []*logpb.LogEntry
					allLogs = append(allLogs, genDG(1337, "foo", "bar", "baz", "kthxbye")...)
					allLogs = append(allLogs, genDG(1341, "qux", "ohai")...)
					allLogs = append(allLogs, genDG(1343, "complete")...)
					tailLog := allLogs[len(allLogs)-1]

					svc.TH = func(req *logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{tailLog},
							State: &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
							},
							Desc: &logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.StreamType_DATAGRAM,
							},
						}, nil
					}

					svc.GH = func(req *logdog.GetRequest) (*logdog.GetResponse, error) {
						if req.State || req.NonContiguous || req.ByteCount != 0 {
							return nil, errors.New("not implemented in test")
						}
						if len(allLogs) == 0 {
							return &logdog.GetResponse{}, nil
						}

						// Identify the requested index.
						var ret []*logpb.LogEntry
						for i, le := range allLogs {
							if le.StreamIndex == uint64(req.Index) {
								ret = allLogs[i:]
								break
							}
						}
						count := int(req.LogCount)
						if count > len(ret) {
							count = len(ret)
						}
						return &logdog.GetResponse{
							Logs: ret[:count],
						}, nil
					}

					Convey(`With a non-partial datagram, returns that datagram.`, func() {
						le, err := s.Tail(c, Complete())
						So(err, ShouldBeNil)
						So(le.StreamIndex, ShouldEqual, 1343)
						So(le.GetDatagram().Partial, ShouldBeNil)
						So(le.GetDatagram().Data, ShouldResemble, []byte("complete"))
					})

					Convey(`Can assemble a set of one partial datagram.`, func() {
						// This is weird, since this doesn't need to be partial at all, but
						// we should handle it gracefully.
						dg := tailLog.GetDatagram()
						dg.Partial = &logpb.Datagram_Partial{
							Index: 0,
							Size:  uint64(len(dg.Data)),
							Last:  true,
						}

						le, err := s.Tail(c, Complete())
						So(err, ShouldBeNil)
						So(le.StreamIndex, ShouldEqual, 1343)
						So(le.GetDatagram().Partial, ShouldBeNil)
						So(le.GetDatagram().Data, ShouldResemble, []byte("complete"))
					})

					Convey(`Can assemble a set of two partial datagrams.`, func() {
						tailLog = allLogs[5]

						le, err := s.Tail(c, Complete())
						So(err, ShouldBeNil)
						So(le.StreamIndex, ShouldEqual, 1341)
						So(le.GetDatagram().Partial, ShouldBeNil)
						So(le.GetDatagram().Data, ShouldResemble, []byte("quxohai"))
					})

					Convey(`With a set of three partial datagrams.`, func() {
						tailLog = allLogs[3]

						Convey(`Will return a fully reassembled datagram.`, func() {
							var ls LogStream
							le, err := s.Tail(c, WithState(&ls), Complete())
							So(err, ShouldBeNil)
							So(le.StreamIndex, ShouldEqual, 1337)
							So(le.GetDatagram().Partial, ShouldBeNil)
							So(le.GetDatagram().Data, ShouldResemble, []byte("foobarbazkthxbye"))

							So(ls, ShouldResemble, LogStream{
								Path: "test/+/a",
								Desc: logpb.LogStreamDescriptor{
									Prefix:     "test",
									Name:       "a",
									StreamType: logpb.StreamType_DATAGRAM,
								},
								State: StreamState{
									Created: now,
								},
							})
						})

						Convey(`Will return an error if the Get fails.`, func() {
							svc.GH = func(req *logdog.GetRequest) (*logdog.GetResponse, error) {
								return nil, grpcutil.Errf(codes.InvalidArgument, "test error")
							}

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "failed to get intermediate logs")
							So(err, ShouldErrLike, "test error")
						})

						Convey(`Will return an error if the Get returns fewer logs than requested.`, func() {
							allLogs = allLogs[0:1]

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "incomplete intermediate logs results")
						})

						Convey(`Will return an error if Get returns non-datagram logs.`, func() {
							allLogs[1].Content = nil

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "is not a datagram")
						})

						Convey(`Will return an error if Get returns non-partial datagram logs.`, func() {
							allLogs[1].GetDatagram().Partial = nil

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "is not partial")
						})

						Convey(`Will return an error if Get returns non-contiguous partial datagrams.`, func() {
							allLogs[1].GetDatagram().Partial.Index = 2

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "does not have a contiguous index")
						})

						Convey(`Will return an error if the chunks declare different sizes.`, func() {
							allLogs[1].GetDatagram().Partial.Size = 0

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "inconsistent datagram size")
						})

						Convey(`Will return an error if the reassembled length exceeds the declared size.`, func() {
							for _, le := range allLogs {
								if p := le.GetDatagram().Partial; p != nil {
									p.Size = 0
								}
							}

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "appending chunk data would exceed the declared size")
						})

						Convey(`Will return an error if the reassembled length doesn't match the declared size.`, func() {
							for _, le := range allLogs {
								if p := le.GetDatagram().Partial; p != nil {
									p.Size = 1024 * 1024
								}
							}

							_, err := s.Tail(c, Complete())
							So(err, ShouldErrLike, "differs from declared length")
						})
					})

					Convey(`When Tail returns a mid-partial datagram.`, func() {
						tailLog = allLogs[4]

						Convey(`If the previous datagram is partial, will return it reassembled.`, func() {
							le, err := s.Tail(c, Complete())
							So(err, ShouldBeNil)
							So(le.StreamIndex, ShouldEqual, 1337)
							So(le.GetDatagram().Partial, ShouldBeNil)
							So(le.GetDatagram().Data, ShouldResemble, []byte("foobarbazkthxbye"))
						})

						Convey(`If the previous datagram is not partial, will return it.`, func() {
							allLogs[3].GetDatagram().Partial = nil

							le, err := s.Tail(c, Complete())
							So(err, ShouldBeNil)
							So(le.StreamIndex, ShouldEqual, 1340)
							So(le.GetDatagram().Partial, ShouldBeNil)
							So(le.GetDatagram().Data, ShouldResemble, []byte("kthxbye"))
						})
					})
				})
			})
		})
	})
}
