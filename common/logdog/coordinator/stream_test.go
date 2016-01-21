// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"net/http"
	"testing"

	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
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

func TestStream(t *testing.T) {
	Convey(`A testing Client`, t, func() {
		ctx := context.Background()
		rt := testRT{}
		httpClient := http.Client{}
		httpClient.Transport = &rt

		config := Config{
			Client:          &httpClient,
			UserAgent:       "testing-user-agent",
			LogsAPIBasePath: "http://example.com",
		}
		client, err := New(config)
		So(err, ShouldBeNil)

		Convey(`Can bind a Stream`, func() {
			s := client.Stream(types.StreamPath("test/+/a"))

			Convey(`Test Get`, func() {
				p := NewGetParams()

				Convey(`Will form a proper Get logs query.`, func() {
					p = p.Logs(nil, 1024).
						NonContiguous().
						Index(1)

					var req *http.Request
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						req = ireq
						return &logs.GetResponse{}, nil
					}

					l, err := s.Get(ctx, p)
					So(err, ShouldBeNil)
					So(l, ShouldBeNil)

					// Validate the HTTP request that we made.
					So(req, ShouldNotBeNil)
					v := req.URL.Query()
					So(v.Get("noncontiguous"), ShouldEqual, "true")
					So(v.Get("path"), ShouldEqual, "test/+/a")
					So(v.Get("index"), ShouldEqual, "1")
					So(v.Get("bytes"), ShouldEqual, "1024")
					So(v.Get("proto"), ShouldEqual, "true")
				})

				Convey(`Will request a specific number of logs if a slice is supplied.`, func() {
					logSlice := make([]*logpb.LogEntry, 64)
					p = p.Logs(logSlice, 32)

					var req *http.Request
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						req = ireq

						return &logs.GetResponse{
							Logs: []*logs.GetLogEntry{
								{Proto: b64Proto(genLog(1337, "ohai"))},
							},
						}, nil
					}

					l, err := s.Get(ctx, p)
					So(err, ShouldBeNil)
					So(l, ShouldResembleV, []*logpb.LogEntry{genLog(1337, "ohai")})

					// It should have populated the input "logSlice".
					So(l, ShouldResembleV, logSlice[:1])

					// Validate the HTTP request that we made.
					So(req, ShouldNotBeNil)
					v := req.URL.Query()
					So(v.Get("count"), ShouldEqual, "64")
					So(v.Get("bytes"), ShouldEqual, "32")
				})

				Convey(`Will request just the state if Logs isn't called.`, func() {
					// Add some log-only parameters to verify that they're not presnet.
					p = p.Index(1337).NonContiguous()

					var req *http.Request
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						req = ireq
						return &logs.GetResponse{}, nil
					}

					l, err := s.Get(ctx, p)
					So(err, ShouldBeNil)
					So(l, ShouldBeNil)

					// Validate the HTTP request that we made.
					So(req, ShouldNotBeNil)
					v := req.URL.Query()
					So(v.Get("path"), ShouldEqual, "test/+/a")
					So(v.Get("count"), ShouldEqual, "-1")
					So(v.Get("proto"), ShouldEqual, "true")

					// Should NOT be set.
					So(v.Get("noncontiguous"), ShouldEqual, "")
					So(v.Get("index"), ShouldEqual, "")
				})

				Convey(`Can decode a full protobuf and state.`, func() {
					st := StreamState{}
					p = p.Logs(nil, 0).State(&st)

					rt.handler = func(*http.Request) (interface{}, error) {
						return &logs.GetResponse{
							Logs: []*logs.GetLogEntry{
								{Proto: b64Proto(genLog(1337, "kthxbye"))},
							},
							State: &logs.LogStreamState{
								Created: timeString(now),
								Updated: timeString(now),
							},
							DescriptorProto: b64Proto(&logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.LogStreamDescriptor_TEXT,
							}),
						}, nil
					}

					l, err := s.Get(ctx, p)
					So(err, ShouldBeNil)
					So(l, ShouldResembleV, []*logpb.LogEntry{genLog(1337, "kthxbye")})
					So(st, ShouldResembleV, StreamState{
						Created: now.UTC(),
						Updated: now.UTC(),
						Descriptor: &logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.LogStreamDescriptor_TEXT,
						},
					})
				})

				Convey(`Will return ErrNoSuchStream if the stream is not found.`, func() {
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						return nil, httpError(http.StatusNotFound)
					}

					_, err := s.Get(ctx, p)
					So(err, ShouldEqual, ErrNoSuchStream)
				})

				Convey(`Will return ErrNoAccess if unauthorized.`, func() {
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						return nil, httpError(http.StatusUnauthorized)
					}

					_, err := s.Get(ctx, p)
					So(err, ShouldEqual, ErrNoAccess)
				})

				Convey(`Will return ErrNoAccess if forbidden.`, func() {
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						return nil, httpError(http.StatusForbidden)
					}

					_, err := s.Get(ctx, p)
					So(err, ShouldEqual, ErrNoAccess)
				})
			})

			Convey(`Test Tail`, func() {
				Convey(`Will form a proper Tail query.`, func() {
					var req *http.Request
					rt.handler = func(ireq *http.Request) (interface{}, error) {
						req = ireq
						return &logs.GetResponse{
							State: &logs.LogStreamState{
								Created: timeString(now),
								Updated: timeString(now),
							},
							DescriptorProto: b64Proto(&logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.LogStreamDescriptor_TEXT,
							}),
							Logs: []*logs.GetLogEntry{
								{Proto: b64Proto(genLog(1337, "kthxbye"))},
							},
						}, nil
					}

					st := StreamState{}
					l, err := s.Tail(ctx, &st)
					So(err, ShouldBeNil)

					// Validate the HTTP request that we made.
					So(req, ShouldNotBeNil)
					v := req.URL.Query()
					So(v.Get("path"), ShouldEqual, "test/+/a")
					So(v.Get("tail"), ShouldEqual, "true")
					So(v.Get("state"), ShouldEqual, "true")

					// Validate that the log and state were returned.
					So(l, ShouldResembleV, genLog(1337, "kthxbye"))
					So(st, ShouldResembleV, StreamState{
						Created: now.UTC(),
						Updated: now.UTC(),
						Descriptor: &logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.LogStreamDescriptor_TEXT,
						},
					})
				})

				Convey(`Will return nil with state if no logs are returned from the endpoint.`, func() {
					rt.handler = func(req *http.Request) (interface{}, error) {
						return &logs.GetResponse{
							State: &logs.LogStreamState{
								Created: timeString(now),
								Updated: timeString(now),
							},
							DescriptorProto: b64Proto(&logpb.LogStreamDescriptor{
								Prefix:     "test",
								Name:       "a",
								StreamType: logpb.LogStreamDescriptor_TEXT,
							}),
						}, nil
					}

					st := StreamState{}
					l, err := s.Tail(ctx, &st)
					So(err, ShouldBeNil)
					So(l, ShouldBeNil)
					So(st, ShouldResembleV, StreamState{
						Created: now.UTC(),
						Updated: now.UTC(),
						Descriptor: &logpb.LogStreamDescriptor{
							Prefix:     "test",
							Name:       "a",
							StreamType: logpb.LogStreamDescriptor_TEXT,
						},
					})
				})

				Convey(`Will error if multiple logs are returned from the endpoint.`, func() {
					rt.handler = func(req *http.Request) (interface{}, error) {
						return &logs.GetResponse{
							State: &logs.LogStreamState{
								Created: timeString(now),
								Updated: timeString(now),
							},
							Logs: []*logs.GetLogEntry{
								{Proto: b64Proto(genLog(1337, "ohai"))},
								{Proto: b64Proto(genLog(1338, "kthxbye"))},
							},
						}, nil
					}

					_, err := s.Tail(ctx, nil)
					So(err, ShouldErrLike, "tail call returned 2 logs")
				})
			})
		})
	})
}
