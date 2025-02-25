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
	"context"
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
)

func genLog(idx int64, id string) *logpb.LogEntry {
	return &logpb.LogEntry{
		StreamIndex: uint64(idx),
		Content: &logpb.LogEntry_Text{
			Text: &logpb.Text{
				Lines: []*logpb.Text_Line{
					{Value: []byte(id)},
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
			Content:     &logpb.LogEntry_Datagram{Datagram: &dg},
		}
	}
	return logs
}

// testStreamLogsService implements just the Get and Tail endpoints,
// instrumented for testing.
type testStreamLogsService struct {
	testLogsServiceBase

	// Get
	GR *logdog.GetRequest
	GH func(*logdog.GetRequest) (*logdog.GetResponse, error)

	// Tail
	TR *logdog.TailRequest
	TH func(*logdog.TailRequest) (*logdog.GetResponse, error)
}

func (s *testStreamLogsService) Get(c context.Context, req *logdog.GetRequest) (*logdog.GetResponse, error) {
	s.GR = req
	if h := s.GH; h != nil {
		return s.GH(req)
	}
	return nil, errors.New("not implemented")
}

func (s *testStreamLogsService) Tail(c context.Context, req *logdog.TailRequest) (*logdog.GetResponse, error) {
	s.TR = req
	if h := s.TH; h != nil {
		return s.TH(req)
	}
	return nil, errors.New("not implemented")
}

func TestStreamGet(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing Client`, t, func(t *ftt.Test) {
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

		t.Run(`Can bind a Stream`, func(t *ftt.Test) {
			s := client.Stream("myproj", "test/+/a")

			t.Run(`Test Get`, func(t *ftt.Test) {
				t.Run(`A default Get query will return logs and no state.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{
								genLog(1337, "ohai"),
								genLog(1338, "kthxbye"),
							},
						}, nil
					}

					l, err := s.Get(c)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, l, should.Match([]*logpb.LogEntry{genLog(1337, "ohai"), genLog(1338, "kthxbye")}))

					// Validate the correct parameters were sent.
					assert.Loosely(t, svc.GR, should.Match(&logdog.GetRequest{
						Project: "myproj",
						Path:    "test/+/a",
					}))
				})

				t.Run(`Will form a proper Get logs query.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{}, nil
					}

					l, err := s.Get(c, NonContiguous(), Index(1))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, l, should.BeNil)

					// Validate the correct parameters were sent.
					assert.Loosely(t, svc.GR, should.Match(&logdog.GetRequest{
						Project:       "myproj",
						Path:          "test/+/a",
						NonContiguous: true,
						Index:         1,
					}))
				})

				t.Run(`Will request a specific number of logs if a constraint is supplied.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{
								genLog(1337, "ohai"),
							},
						}, nil
					}

					l, err := s.Get(c, LimitCount(64), LimitBytes(32))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, l, should.Match([]*logpb.LogEntry{genLog(1337, "ohai")}))

					// Validate the HTTP request that we made.
					assert.Loosely(t, svc.GR, should.Match(&logdog.GetRequest{
						Project:   "myproj",
						Path:      "test/+/a",
						LogCount:  64,
						ByteCount: 32,
					}))
				})

				t.Run(`Can decode a full protobuf and state.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{
								genLog(1337, "kthxbye"),
							},
							State: &logdog.LogStreamState{
								Created: timestamppb.New(now),
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, l, should.Match([]*logpb.LogEntry{genLog(1337, "kthxbye")}))
					assert.That(t, &ls, should.Match(&LogStream{
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
					}))
				})

				t.Run(`Will return ErrNoSuchStream if the stream is not found.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.NotFound, "not found")
					}

					_, err := s.Get(c)
					assert.Loosely(t, err, should.Equal(ErrNoSuchStream))
				})

				t.Run(`Will return ErrNoAccess if unauthenticated.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.Unauthenticated, "unauthenticated")
					}

					_, err := s.Get(c)
					assert.Loosely(t, err, should.Equal(ErrNoAccess))
				})

				t.Run(`Will return ErrNoAccess if permission is denied.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.PermissionDenied, "boom")
					}

					_, err := s.Get(c)
					assert.Loosely(t, err, should.Equal(ErrNoAccess))
				})
			})

			t.Run(`Test State`, func(t *ftt.Test) {
				t.Run(`Will request just the state if asked.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Project: "myproj",
							Desc: &logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.StreamType_TEXT,
							},
							State: &logdog.LogStreamState{
								Created: timestamppb.New(now),
							},
						}, nil
					}

					l, err := s.State(c)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, l, should.Match(&LogStream{
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
					}))

					// Validate the HTTP request that we made.
					assert.Loosely(t, svc.GR, should.Match(&logdog.GetRequest{
						Project:  "myproj",
						Path:     "test/+/a",
						LogCount: -1,
						State:    true,
					}))
				})

				t.Run(`Will return ErrNoSuchStream if the stream is not found.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.NotFound, "not found")
					}

					_, err := s.State(c)
					assert.Loosely(t, err, should.Equal(ErrNoSuchStream))
				})

				t.Run(`Will return ErrNoAccess if unauthenticated.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.Unauthenticated, "unauthenticated")
					}

					_, err := s.State(c)
					assert.Loosely(t, err, should.Equal(ErrNoAccess))
				})

				t.Run(`Will return ErrNoAccess if permission is denied.`, func(t *ftt.Test) {
					svc.GH = func(*logdog.GetRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.PermissionDenied, "boom")
					}

					_, err := s.State(c)
					assert.Loosely(t, err, should.Equal(ErrNoAccess))
				})
			})

			t.Run(`Test Tail`, func(t *ftt.Test) {
				t.Run(`Will form a proper Tail query.`, func(t *ftt.Test) {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Project: "myproj",
							State: &logdog.LogStreamState{
								Created: timestamppb.New(now),
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
					assert.Loosely(t, err, should.BeNil)

					// Validate the HTTP request that we made.
					assert.Loosely(t, svc.TR, should.Match(&logdog.TailRequest{
						Project: "myproj",
						Path:    "test/+/a",
						State:   true,
					}))

					// Validate that the log and state were returned.
					assert.Loosely(t, l, should.Match(genLog(1337, "kthxbye")))
					assert.That(t, &ls, should.Match(&LogStream{
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
					}))
				})

				t.Run(`Will return nil with state if no logs are returned from the endpoint.`, func(t *ftt.Test) {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Project: "myproj",
							State: &logdog.LogStreamState{
								Created: timestamppb.New(now),
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
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, l, should.BeNil)
					assert.That(t, &ls, should.Match(&LogStream{
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
					}))
				})

				t.Run(`Will error if multiple logs are returned from the endpoint.`, func(t *ftt.Test) {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							State: &logdog.LogStreamState{
								Created: timestamppb.New(now),
							},
							Logs: []*logpb.LogEntry{
								genLog(1337, "ohai"),
								genLog(1338, "kthxbye"),
							},
						}, nil
					}

					_, err := s.Tail(c)
					assert.Loosely(t, err, should.ErrLike("tail call returned 2 logs"))
				})

				t.Run(`Will return ErrNoSuchStream if the stream is not found.`, func(t *ftt.Test) {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.NotFound, "not found")
					}

					_, err := s.Tail(c)
					assert.Loosely(t, err, should.Equal(ErrNoSuchStream))
				})

				t.Run(`Will return ErrNoAccess if unauthenticated.`, func(t *ftt.Test) {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.Unauthenticated, "unauthenticated")
					}

					_, err := s.Tail(c)
					assert.Loosely(t, err, should.Equal(ErrNoAccess))
				})

				t.Run(`Will return ErrNoAccess if permission is denied.`, func(t *ftt.Test) {
					svc.TH = func(*logdog.TailRequest) (*logdog.GetResponse, error) {
						return nil, status.Error(codes.PermissionDenied, "boom")
					}

					_, err := s.Tail(c)
					assert.Loosely(t, err, should.Equal(ErrNoAccess))
				})

				t.Run(`When requesting complete streams`, func(t *ftt.Test) {
					var allLogs []*logpb.LogEntry
					allLogs = append(allLogs, genDG(1337, "foo", "bar", "baz", "kthxbye")...)
					allLogs = append(allLogs, genDG(1341, "qux", "ohai")...)
					allLogs = append(allLogs, genDG(1343, "complete")...)
					tailLog := allLogs[len(allLogs)-1]

					svc.TH = func(req *logdog.TailRequest) (*logdog.GetResponse, error) {
						return &logdog.GetResponse{
							Logs: []*logpb.LogEntry{tailLog},
							State: &logdog.LogStreamState{
								Created: timestamppb.New(now),
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

					t.Run(`With a non-partial datagram, returns that datagram.`, func(t *ftt.Test) {
						le, err := s.Tail(c, Complete())
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, le.StreamIndex, should.Equal[uint64](1343))
						assert.Loosely(t, le.GetDatagram().Partial, should.BeNil)
						assert.Loosely(t, le.GetDatagram().Data, should.Match([]byte("complete")))
					})

					t.Run(`Can assemble a set of one partial datagram.`, func(t *ftt.Test) {
						// This is weird, since this doesn't need to be partial at all, but
						// we should handle it gracefully.
						dg := tailLog.GetDatagram()
						dg.Partial = &logpb.Datagram_Partial{
							Index: 0,
							Size:  uint64(len(dg.Data)),
							Last:  true,
						}

						le, err := s.Tail(c, Complete())
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, le.StreamIndex, should.Equal[uint64](1343))
						assert.Loosely(t, le.GetDatagram().Partial, should.BeNil)
						assert.Loosely(t, le.GetDatagram().Data, should.Match([]byte("complete")))
					})

					t.Run(`Can assemble a set of two partial datagrams.`, func(t *ftt.Test) {
						tailLog = allLogs[5]

						le, err := s.Tail(c, Complete())
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, le.StreamIndex, should.Equal[uint64](1341))
						assert.Loosely(t, le.GetDatagram().Partial, should.BeNil)
						assert.Loosely(t, le.GetDatagram().Data, should.Match([]byte("quxohai")))
					})

					t.Run(`With a set of three partial datagrams.`, func(t *ftt.Test) {
						tailLog = allLogs[3]

						t.Run(`Will return a fully reassembled datagram.`, func(t *ftt.Test) {
							var ls LogStream
							le, err := s.Tail(c, WithState(&ls), Complete())
							assert.Loosely(t, err, should.BeNil)
							assert.That(t, le.StreamIndex, should.Equal[uint64](1337))
							assert.Loosely(t, le.GetDatagram().Partial, should.BeNil)
							assert.Loosely(t, le.GetDatagram().Data, should.Match([]byte("foobarbazkthxbye")))

							assert.That(t, &ls, should.Match(&LogStream{
								Path: "test/+/a",
								Desc: logpb.LogStreamDescriptor{
									Prefix:     "test",
									Name:       "a",
									StreamType: logpb.StreamType_DATAGRAM,
								},
								State: StreamState{
									Created: now,
								},
							}))
						})

						t.Run(`Will return an error if the Get fails.`, func(t *ftt.Test) {
							svc.GH = func(req *logdog.GetRequest) (*logdog.GetResponse, error) {
								return nil, status.Errorf(codes.InvalidArgument, "test error")
							}

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("failed to get intermediate logs"))
							assert.Loosely(t, err, should.ErrLike("test error"))
						})

						t.Run(`Will return an error if the Get returns fewer logs than requested.`, func(t *ftt.Test) {
							allLogs = allLogs[0:1]

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("incomplete intermediate logs results"))
						})

						t.Run(`Will return an error if Get returns non-datagram logs.`, func(t *ftt.Test) {
							allLogs[1].Content = nil

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("is not a datagram"))
						})

						t.Run(`Will return an error if Get returns non-partial datagram logs.`, func(t *ftt.Test) {
							allLogs[1].GetDatagram().Partial = nil

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("is not partial"))
						})

						t.Run(`Will return an error if Get returns non-contiguous partial datagrams.`, func(t *ftt.Test) {
							allLogs[1].GetDatagram().Partial.Index = 2

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("does not have a contiguous index"))
						})

						t.Run(`Will return an error if the chunks declare different sizes.`, func(t *ftt.Test) {
							allLogs[1].GetDatagram().Partial.Size = 0

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("inconsistent datagram size"))
						})

						t.Run(`Will return an error if the reassembled length exceeds the declared size.`, func(t *ftt.Test) {
							for _, le := range allLogs {
								if p := le.GetDatagram().Partial; p != nil {
									p.Size = 0
								}
							}

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("appending chunk data would exceed the declared size"))
						})

						t.Run(`Will return an error if the reassembled length doesn't match the declared size.`, func(t *ftt.Test) {
							for _, le := range allLogs {
								if p := le.GetDatagram().Partial; p != nil {
									p.Size = 1024 * 1024
								}
							}

							_, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.ErrLike("differs from declared length"))
						})
					})

					t.Run(`When Tail returns a mid-partial datagram.`, func(t *ftt.Test) {
						tailLog = allLogs[4]

						t.Run(`If the previous datagram is partial, will return it reassembled.`, func(t *ftt.Test) {
							le, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.BeNil)
							assert.That(t, le.StreamIndex, should.Equal[uint64](1337))
							assert.Loosely(t, le.GetDatagram().Partial, should.BeNil)
							assert.Loosely(t, le.GetDatagram().Data, should.Match([]byte("foobarbazkthxbye")))
						})

						t.Run(`If the previous datagram is not partial, will return it.`, func(t *ftt.Test) {
							allLogs[3].GetDatagram().Partial = nil

							le, err := s.Tail(c, Complete())
							assert.Loosely(t, err, should.BeNil)
							assert.That(t, le.StreamIndex, should.Equal[uint64](1340))
							assert.Loosely(t, le.GetDatagram().Partial, should.BeNil)
							assert.Loosely(t, le.GetDatagram().Data, should.Match([]byte("kthxbye")))
						})
					})
				})
			})
		})
	})
}
