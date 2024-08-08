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

package e2etest

import (
	"context"
	"encoding/hex"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/prpctest"

	"go.chromium.org/luci/grpc/prpc"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type service struct {
	R          *HelloReply
	err        error
	outgoingMD metadata.MD

	sleep func() time.Duration

	m            sync.Mutex
	incomingMD   metadata.MD
	incomingPeer *peer.Peer
}

func (s *service) Greet(ctx context.Context, req *HelloRequest) (*HelloReply, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.m.Lock()
	s.incomingMD = md.Copy()
	s.incomingPeer, _ = peer.FromContext(ctx)
	var sleep time.Duration
	if s.sleep != nil {
		sleep = s.sleep()
	}
	s.m.Unlock()

	time.Sleep(sleep)

	if s.outgoingMD != nil {
		if err := prpc.SetHeader(ctx, s.outgoingMD); err != nil {
			return nil, status.Errorf(codes.Internal, "%s", err)
		}
	}

	return s.R, s.err
}

func (s *service) getIncomingMD() metadata.MD {
	s.m.Lock()
	defer s.m.Unlock()
	return s.incomingMD
}

func (s *service) getIncomingPeer() *peer.Peer {
	s.m.Lock()
	defer s.m.Unlock()
	return s.incomingPeer
}

func newTestClient(ctx context.Context, svc *service, opts *prpc.Options) (*prpctest.Server, HelloClient) {
	ts := prpctest.Server{}
	RegisterHelloServer(&ts, svc)
	ts.Start(ctx)

	prpcClient, err := ts.NewClientWithOptions(opts)
	if err != nil {
		panic(err)
	}

	// Use a new transport each time to reduce interference of tests with one
	// another. Also increase transport-level timeouts from defaults since tests
	// can be quite slow on bots.
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   300 * time.Second,
			KeepAlive: 300 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       900 * time.Second,
		TLSHandshakeTimeout:   100 * time.Second,
		ExpectContinueTimeout: 10 * time.Second,
	}

	prpcClient.C = &http.Client{
		Transport: auth.NewModifyingTransport(transport, func(r *http.Request) error {
			r.AddCookie(&http.Cookie{
				Name:  "cookie_1",
				Value: "value_1",
			})
			r.AddCookie(&http.Cookie{
				Name:  "cookie_2",
				Value: "value_2",
			})
			return nil
		}),
	}

	ts.EnableResponseCompression = true
	prpcClient.EnableRequestCompression = true

	return &ts, NewHelloClient(prpcClient)
}

func TestEndToEnd(t *testing.T) {
	t.Parallel()

	Convey(`A client/server for the Greet service`, t, func() {
		ctx := gologger.StdConfig.Use(context.Background())
		svc := service{
			sleep: func() time.Duration { return time.Millisecond },
		}
		ts, client := newTestClient(ctx, &svc, nil)
		defer ts.Close()

		Convey(`Can round-trip a hello message`, func() {
			svc.R = &HelloReply{Message: "sup"}

			resp, err := client.Greet(ctx, &HelloRequest{Name: "round-trip"})
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)
		})

		Convey(`Can send a giant message with compression`, func() {
			svc.R = &HelloReply{Message: "sup"}

			msg := make([]byte, 1024*1024)
			_, err := rand.Read(msg)
			So(err, ShouldBeNil)

			resp, err := client.Greet(ctx, &HelloRequest{Name: hex.EncodeToString(msg)})
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)
		})

		Convey(`Can receive a giant message with compression`, func() {
			msg := make([]byte, 1024*1024)
			_, err := rand.Read(msg)
			So(err, ShouldBeNil)

			svc.R = &HelloReply{Message: hex.EncodeToString(msg)}

			resp, err := client.Greet(ctx, &HelloRequest{Name: "hi"})
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)
		})

		Convey(`Can round-trip status details`, func() {
			detail := &errdetails.DebugInfo{Detail: "x"}

			s := status.New(codes.AlreadyExists, "already exists")
			s, err := s.WithDetails(detail)
			So(err, ShouldBeNil)
			svc.err = s.Err()

			_, err = client.Greet(ctx, &HelloRequest{Name: "round-trip"})
			details := status.Convert(err).Details()
			So(details, ShouldResembleProto, []any{detail})
		})

		Convey(`Can handle non-trivial metadata`, func() {
			md := metadata.New(nil)
			md.Append("MultiVAL-KEY", "val 1", "val 2")
			md.Append("binary-BIN", string([]byte{0, 1, 2, 3}))

			svc.R = &HelloReply{Message: "sup"}
			svc.outgoingMD = md

			var respMD metadata.MD

			ctx = metadata.NewOutgoingContext(ctx, md)
			resp, err := client.Greet(ctx, &HelloRequest{Name: "round-trip"}, grpc.Header(&respMD))
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)

			So(svc.getIncomingMD(), ShouldResemble, metadata.MD{
				":authority":   {ts.Host},
				"binary-bin":   {string([]byte{0, 1, 2, 3})},
				"cookie":       {"cookie_1=value_1; cookie_2=value_2"},
				"host":         {strings.TrimPrefix(ts.HTTP.URL, "http://")},
				"multival-key": {"val 1", "val 2"},
				"user-agent":   {prpc.DefaultUserAgent},
			})

			So(respMD, ShouldResemble, metadata.MD{
				"binary-bin":   {string([]byte{0, 1, 2, 3})},
				"multival-key": {"val 1", "val 2"},
			})
		})

		Convey(`Populates peer`, func() {
			svc.R = &HelloReply{Message: "sup"}
			_, err := client.Greet(ctx, &HelloRequest{Name: "round-trip"})
			So(err, ShouldBeRPCOK)

			peer := svc.getIncomingPeer()
			So(peer, ShouldNotBeNil)
			So(peer.Addr.String(), ShouldStartWith, "127.0.0.1:")
		})
	})
}

func TestTimeouts(t *testing.T) {
	t.Parallel()

	Convey(`A client/server for the Greet service`, t, func() {
		ctx := gologger.StdConfig.Use(context.Background())
		svc := service{R: &HelloReply{Message: "sup"}}
		ts, client := newTestClient(ctx, &svc, &prpc.Options{
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Millisecond,
						Retries: 5,
					},
				}
			},
			PerRPCTimeout: 5 * time.Millisecond,
		})
		defer ts.Close()

		Convey(`Gives up after N retries`, func() {
			svc.sleep = func() time.Duration {
				return 500 * time.Millisecond // much larger than the per-RPC timeout
			}

			_, err := client.Greet(ctx, &HelloRequest{})
			So(err, ShouldBeRPCDeadlineExceeded)
		})

		Convey(`Succeeds after N retries`, func() {
			attempt := 0
			svc.sleep = func() time.Duration {
				attempt += 1
				if attempt > 3 {
					return 0
				}
				return 500 * time.Millisecond
			}

			_, err := client.Greet(ctx, &HelloRequest{})
			So(err, ShouldBeRPCOK)
		})

		Convey(`Gives up on overall timeout`, func() {
			svc.sleep = func() time.Duration {
				return 500 * time.Millisecond // much larger than the per-RPC timeout
			}

			ctx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
			defer cancel()

			_, err := client.Greet(ctx, &HelloRequest{})
			So(err, ShouldBeRPCDeadlineExceeded)
		})

		Convey(`Respected DEADLINE_EXCEEDED response code`, func() {
			svc.err = status.Errorf(codes.DeadlineExceeded, "internal deadline exceeded")

			_, err := client.Greet(ctx, &HelloRequest{})
			So(err, ShouldBeRPCDeadlineExceeded)
		})
	})
}

func TestVerySmallTimeouts(t *testing.T) {
	t.Parallel()

	Convey(`A client/server for the Greet service`, t, func() {
		ctx := gologger.StdConfig.Use(context.Background())
		svc := service{}
		ts, client := newTestClient(ctx, &svc, &prpc.Options{
			PerRPCTimeout: 10 * time.Millisecond,
		})
		defer ts.Close()

		// There should be either no error or DeadlineExceeded error (depending on
		// how speedy is the test runner). There should never be any other errors.
		// This test is inherently non-deterministic since it depends on various
		// places in net/http network guts that can abort the connection.

		Convey(`Round-trip a hello message`, func() {
			svc.R = &HelloReply{Message: "sup"}

			_, err := client.Greet(ctx, &HelloRequest{Name: "round-trip"})
			if err != nil {
				So(err, ShouldBeRPCDeadlineExceeded)
			}
		})

		Convey(`Send a giant message with compression`, func() {
			svc.R = &HelloReply{Message: "sup"}

			msg := make([]byte, 1024*1024)
			_, err := rand.Read(msg)
			So(err, ShouldBeNil)

			_, err = client.Greet(ctx, &HelloRequest{Name: hex.EncodeToString(msg)})
			if err != nil {
				So(err, ShouldBeRPCDeadlineExceeded)
			}
		})

		Convey(`Receive a giant message with compression`, func() {
			msg := make([]byte, 1024*1024)
			_, err := rand.Read(msg)
			So(err, ShouldBeNil)

			svc.R = &HelloReply{Message: hex.EncodeToString(msg)}

			_, err = client.Greet(ctx, &HelloRequest{Name: "hi"})
			if err != nil {
				So(err, ShouldBeRPCDeadlineExceeded)
			}
		})
	})
}
