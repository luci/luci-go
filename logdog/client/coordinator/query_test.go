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
	"fmt"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"

	. "github.com/smartystreets/goconvey/convey"
)

type testQueryLogsService struct {
	testLogsServiceBase

	LR logdog.QueryRequest
	H  func(*logdog.QueryRequest) (*logdog.QueryResponse, error)
}

func (s *testQueryLogsService) Query(c context.Context, req *logdog.QueryRequest) (*logdog.QueryResponse, error) {
	s.LR = *req
	if h := s.H; h != nil {
		return s.H(req)
	}
	return nil, errors.New("not implemented")
}

func gen(name string, state *logdog.LogStreamState) *logdog.QueryResponse_Stream {
	return &logdog.QueryResponse_Stream{
		Path:  fmt.Sprintf("test/+/%s", name),
		State: state,
		Desc: &logpb.LogStreamDescriptor{
			Prefix: "test",
			Name:   name,
		},
	}
}

func shouldHaveLogStreams(actual interface{}, expected ...interface{}) string {
	a := actual.([]*LogStream)

	aList := make([]string, len(a))
	for i, ls := range a {
		aList[i] = string(ls.Path)
	}

	eList := make([]string, len(expected))
	for i, exp := range expected {
		eList[i] = exp.(string)
	}

	return ShouldResemble(aList, eList)
}

func TestClientQuery(t *testing.T) {
	t.Parallel()

	Convey(`A testing Client`, t, func() {
		now := testclock.TestTimeLocal
		c := context.Background()

		ts := prpctest.Server{}
		svc := testQueryLogsService{}
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

		Convey(`When making a query request`, func() {
			const project = types.ProjectName("myproj")
			const path = "**/+/**"
			q := QueryOptions{
				Tags: map[string]string{
					"foo": "bar",
					"baz": "qux",
				},
				ContentType: "application/text",
				Before:      now,
				After:       now,
				Purged:      Both,
				State:       true,
			}

			st := logdog.LogStreamState{
				Created: google.NewTimestamp(now),
			}

			var results []*LogStream
			accumulate := func(s *LogStream) bool {
				results = append(results, s)
				return true
			}

			Convey(`Can accumulate results across queries.`, func() {
				// This handler will return a single query per request, as well as a
				// non-empty Next pointer for the next query element. It progresses
				// "a" => "b" => "final" => "".
				svc.H = func(req *logdog.QueryRequest) (*logdog.QueryResponse, error) {
					r := logdog.QueryResponse{
						Project: string(project),
					}

					switch req.Next {
					case "":
						r.Streams = append(r.Streams, gen("a", &st))
						r.Next = "b"
					case "b":
						r.Streams = append(r.Streams, gen("b", &st))
						r.Next = "final"
					case "final":
						r.Streams = append(r.Streams, gen("final", &st))
					default:
						return nil, errors.New("invalid cursor")
					}
					return &r, nil
				}

				So(client.Query(c, project, path, q, accumulate), ShouldBeNil)
				So(results, shouldHaveLogStreams, "test/+/a", "test/+/b", "test/+/final")
			})

			Convey(`Will stop invoking the callback if it returns false.`, func() {
				// This handler will return three query results, "a", "b", and "c".
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return &logdog.QueryResponse{
						Streams: []*logdog.QueryResponse_Stream{
							gen("a", &st),
							gen("b", &st),
							gen("c", &st),
						},
						Next: "infiniteloop",
					}, nil
				}

				accumulate = func(s *LogStream) bool {
					results = append(results, s)
					return len(results) < 3
				}
				So(client.Query(c, project, path, q, accumulate), ShouldBeNil)
				So(results, shouldHaveLogStreams, "test/+/a", "test/+/b", "test/+/c")
			})

			Convey(`Will properly handle state and protobuf deserialization.`, func() {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return &logdog.QueryResponse{
						Streams: []*logdog.QueryResponse_Stream{
							gen("a", &logdog.LogStreamState{
								Created: google.NewTimestamp(now),
							}),
						},
					}, nil
				}

				So(client.Query(c, project, path, q, accumulate), ShouldBeNil)
				So(results, shouldHaveLogStreams, "test/+/a")
				So(results[0], ShouldResemble, &LogStream{
					Path: "test/+/a",
					Desc: logpb.LogStreamDescriptor{Prefix: "test", Name: "a"},
					State: StreamState{
						Created: now.UTC(),
					},
				})
			})

			Convey(`Can query for stream types`, func() {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return &logdog.QueryResponse{}, nil
				}

				Convey(`Text`, func() {
					q.StreamType = Text
					So(client.Query(c, project, path, q, accumulate), ShouldBeNil)
					So(svc.LR.StreamType, ShouldResemble, &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_TEXT})
				})

				Convey(`Binary`, func() {
					q.StreamType = Binary
					So(client.Query(c, project, path, q, accumulate), ShouldBeNil)
					So(svc.LR.StreamType, ShouldResemble, &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_BINARY})
				})

				Convey(`Datagram`, func() {
					q.StreamType = Datagram
					So(client.Query(c, project, path, q, accumulate), ShouldBeNil)
					So(svc.LR.StreamType, ShouldResemble, &logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_DATAGRAM})
				})
			})

			Convey(`Will return ErrNoAccess if unauthenticated.`, func() {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return nil, grpcutil.Unauthenticated
				}

				So(client.Query(c, project, path, q, accumulate), ShouldEqual, ErrNoAccess)
			})

			Convey(`Will return ErrNoAccess if permission denied.`, func() {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return nil, grpcutil.Unauthenticated
				}

				So(client.Query(c, project, path, q, accumulate), ShouldEqual, ErrNoAccess)
			})
		})
	})
}
