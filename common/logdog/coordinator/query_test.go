// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"golang.org/x/net/context"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestClientQuery(t *testing.T) {
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

		Convey(`When making a query request`, func() {
			q := Query{
				Path: "**/+/**",
				Tags: map[string]string{
					"foo": "bar",
					"baz": "qux",
				},
				ContentType: "application/text",
				Before:      testclock.TestTimeLocal,
				After:       testclock.TestTimeLocal,
				Terminated:  Yes,
				Archived:    No,
				Purged:      Both,
				State:       true,
			}

			var results []*QueryStream
			accumulate := func(s *QueryStream) bool {
				results = append(results, s)
				return true
			}

			Convey(`Will contact the correct endpoint using the correct user agent.`, func() {
				var req *http.Request
				rt.handler = func(ireq *http.Request) (interface{}, error) {
					req = ireq
					return nil, nil
				}

				So(client.Query(ctx, &q, accumulate), ShouldBeNil)
				So(req.UserAgent(), ShouldContainSubstring, "testing-user-agent")
				So(req.URL.String(), ShouldStartWith, "http://example.com")
			})

			Convey(`Can accumulate results across queries.`, func() {
				// This handler will return a single query per request, as well as a
				// non-empty Next pointer for the next query element. It progresses
				// "a" => "b" => "final" => "".
				rt.handler = func(req *http.Request) (interface{}, error) {
					body, err := ioutil.ReadAll(req.Body)
					if err != nil {
						return nil, err
					}

					qr := logs.QueryRequest{}
					if err := json.Unmarshal(body, &qr); err != nil {
						return nil, err
					}

					r := logs.QueryResponse{}
					switch qr.Next {
					case "":
						r.Streams = append(r.Streams, gen("a", nil, nil))
						r.Next = "b"
					case "b":
						r.Streams = append(r.Streams, gen("b", nil, nil))
						r.Next = "final"
					case "final":
						r.Streams = append(r.Streams, gen("final", nil, nil))
					default:
						return nil, errors.New("invalid cursor")
					}

					return &r, nil
				}

				So(client.Query(ctx, &q, accumulate), ShouldBeNil)
				So(results, ShouldResembleV, []*QueryStream{
					{Path: "test/+/a"},
					{Path: "test/+/b"},
					{Path: "test/+/final"},
				})
			})

			Convey(`Will stop invoking the callback if it returns false.`, func() {
				// This handler will return three query results, "a", "b", and "c".
				rt.handler = func(req *http.Request) (interface{}, error) {
					return &logs.QueryResponse{
						Streams: []*logs.QueryResponseStream{
							gen("a", nil, nil),
							gen("b", nil, nil),
							gen("c", nil, nil),
						},
						Next: "infiniteloop",
					}, nil
				}

				accumulate = func(s *QueryStream) bool {
					results = append(results, s)
					return len(results) < 3
				}
				So(client.Query(ctx, &q, accumulate), ShouldBeNil)
				So(results, ShouldResembleV, []*QueryStream{
					{Path: "test/+/a"},
					{Path: "test/+/b"},
					{Path: "test/+/c"},
				})
			})

			Convey(`Will properly handle state and protobuf deserialization.`, func() {
				pb := protocol.LogStreamDescriptor{
					Prefix: "test",
					Name:   "a",
				}
				d, err := proto.Marshal(&pb)
				So(err, ShouldBeNil)

				rt.handler = func(ireq *http.Request) (interface{}, error) {
					return &logs.QueryResponse{
						Streams: []*logs.QueryResponseStream{
							gen("a", &pb, &logs.LogStreamState{
								Created: timeString(now),
								Updated: timeString(now),
							}),
						},
					}, nil
				}

				So(client.Query(ctx, &q, accumulate), ShouldBeNil)
				So(results, ShouldResembleV, []*QueryStream{
					{Path: "test/+/a", DescriptorProto: d, State: &StreamState{
						Created: now.UTC(),
						Updated: now.UTC(),
					}},
				})

				desc, err := results[0].Descriptor()
				So(err, ShouldBeNil)
				So(desc, ShouldResembleV, &pb)
			})

			Convey(`Will return ErrNoAccess if unauthorized.`, func() {
				rt.handler = func(ireq *http.Request) (interface{}, error) {
					return nil, httpError(http.StatusUnauthorized)
				}

				So(client.Query(ctx, &q, accumulate), ShouldEqual, ErrNoAccess)
			})

			Convey(`Will return ErrNoAccess if forbidden.`, func() {
				rt.handler = func(ireq *http.Request) (interface{}, error) {
					return nil, httpError(http.StatusUnauthorized)
				}

				So(client.Query(ctx, &q, accumulate), ShouldEqual, ErrNoAccess)
			})
		})
	})
}
