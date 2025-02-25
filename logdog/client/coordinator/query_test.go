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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"

	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/api/logpb"
)

type testQueryLogsService struct {
	testLogsServiceBase

	LR *logdog.QueryRequest
	H  func(*logdog.QueryRequest) (*logdog.QueryResponse, error)
}

func (s *testQueryLogsService) Query(c context.Context, req *logdog.QueryRequest) (*logdog.QueryResponse, error) {
	s.LR = proto.Clone(req).(*logdog.QueryRequest)
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

func shouldHaveLogStreams(expected ...string) comparison.Func[[]*LogStream] {
	return func(actual []*LogStream) *failure.Summary {
		aList := make([]string, len(actual))
		for i, ls := range actual {
			aList[i] = string(ls.Path)
		}

		ret := should.Match(expected)(aList)
		if ret != nil {
			ret.Comparison.Name = "shouldHaveLogStreams"
		}
		return ret
	}
}

func TestClientQuery(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing Client`, t, func(t *ftt.Test) {
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

		t.Run(`When making a query request`, func(t *ftt.Test) {
			const project = "myproj"
			const path = "**/+/**"
			q := QueryOptions{
				Tags: map[string]string{
					"foo": "bar",
					"baz": "qux",
				},
				ContentType: "application/text",
				Purged:      Both,
				State:       true,
			}

			st := logdog.LogStreamState{
				Created: timestamppb.New(now),
			}

			var results []*LogStream
			accumulate := func(s *LogStream) bool {
				results = append(results, s)
				return true
			}

			t.Run(`Can accumulate results across queries.`, func(t *ftt.Test) {
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

				assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.BeNil)
				assert.Loosely(t, results, shouldHaveLogStreams("test/+/a", "test/+/b", "test/+/final"))
			})

			t.Run(`Will stop invoking the callback if it returns false.`, func(t *ftt.Test) {
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
				assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.BeNil)
				assert.Loosely(t, results, shouldHaveLogStreams("test/+/a", "test/+/b", "test/+/c"))
			})

			t.Run(`Will properly handle state and protobuf deserialization.`, func(t *ftt.Test) {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return &logdog.QueryResponse{
						Streams: []*logdog.QueryResponse_Stream{
							gen("a", &logdog.LogStreamState{
								Created: timestamppb.New(now),
							}),
						},
					}, nil
				}

				assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.BeNil)
				assert.Loosely(t, results, shouldHaveLogStreams("test/+/a"))
				assert.Loosely(t, results[0], should.Match(&LogStream{
					Path: "test/+/a",
					Desc: logpb.LogStreamDescriptor{Prefix: "test", Name: "a"},
					State: StreamState{
						Created: now.UTC(),
					},
				}))
			})

			t.Run(`Can query for stream types`, func(t *ftt.Test) {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return &logdog.QueryResponse{}, nil
				}

				t.Run(`Text`, func(t *ftt.Test) {
					q.StreamType = Text
					assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.BeNil)
					assert.Loosely(t, svc.LR.StreamType, should.Match(&logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_TEXT}))
				})

				t.Run(`Binary`, func(t *ftt.Test) {
					q.StreamType = Binary
					assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.BeNil)
					assert.Loosely(t, svc.LR.StreamType, should.Match(&logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_BINARY}))
				})

				t.Run(`Datagram`, func(t *ftt.Test) {
					q.StreamType = Datagram
					assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.BeNil)
					assert.Loosely(t, svc.LR.StreamType, should.Match(&logdog.QueryRequest_StreamTypeFilter{Value: logpb.StreamType_DATAGRAM}))
				})
			})

			t.Run(`Will return ErrNoAccess if unauthenticated.`, func(t *ftt.Test) {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return nil, status.Error(codes.Unauthenticated, "unauthenticated")
				}

				assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.Equal(ErrNoAccess))
			})

			t.Run(`Will return ErrNoAccess if permission denied.`, func(t *ftt.Test) {
				svc.H = func(*logdog.QueryRequest) (*logdog.QueryResponse, error) {
					return nil, status.Error(codes.Unauthenticated, "unauthenticated")
				}

				assert.Loosely(t, client.Query(c, project, path, q, accumulate), should.Equal(ErrNoAccess))
			})
		})
	})
}
