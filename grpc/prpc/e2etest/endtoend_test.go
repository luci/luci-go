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
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/grpc/prpc/internal/testpb"
)

type service struct {
	testpb.UnimplementedGreeterServer

	R          *testpb.HelloReply
	err        error
	outgoingMD metadata.MD

	sleep func() time.Duration

	m              sync.Mutex
	incomingMD     metadata.MD
	incomingPeer   *peer.Peer
	incomingFields []string
}

func (s *service) SayHello(ctx context.Context, req *testpb.HelloRequest) (*testpb.HelloReply, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	s.m.Lock()
	s.incomingMD = md.Copy()
	s.incomingPeer, _ = peer.FromContext(ctx)
	s.incomingFields = append([]string(nil), req.Fields.GetPaths()...)
	var sleep time.Duration
	if s.sleep != nil {
		sleep = s.sleep()
	}
	s.m.Unlock()

	select {
	case <-ctx.Done():
		return nil, status.FromContextError(ctx.Err()).Err()
	case <-time.After(sleep):
	}

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

func (s *service) getIncomingFields() []string {
	s.m.Lock()
	defer s.m.Unlock()
	return s.incomingFields
}

func newTestClient(ctx context.Context, svc *service, opts *prpc.Options) (*prpctest.Server, *prpc.Client, testpb.GreeterClient) {
	ts := prpctest.Server{}
	testpb.RegisterGreeterServer(&ts, svc)
	ts.Start(ctx)

	prpcClient, err := ts.NewClientWithOptions(opts)
	if err != nil {
		panic(err)
	}

	// Setup cookies to verify they are accessible as metadata.
	jar, err := cookiejar.New(nil)
	if err != nil {
		panic(err)
	}
	jar.SetCookies(&url.URL{Scheme: "http", Host: ts.Host, Path: "/"}, []*http.Cookie{
		{
			Name:  "cookie_1",
			Value: "value_1",
		},
		{
			Name:  "cookie_2",
			Value: "value_2",
		},
	})

	prpcClient.C = &http.Client{
		Jar:       jar,
		Transport: prpcClient.C.Transport, // inherit httptest transport
	}

	ts.ResponseCompression = prpc.CompressAlways
	prpcClient.EnableRequestCompression = true

	return &ts, prpcClient, testpb.NewGreeterClient(prpcClient)
}

func TestEndToEndBinary(t *testing.T) {
	t.Parallel()
	endToEndTest(t, prpc.FormatBinary)
}

func TestEndToEndJSON(t *testing.T) {
	t.Parallel()
	endToEndTest(t, prpc.FormatJSONPB)
}

func TestEndToEndText(t *testing.T) {
	t.Parallel()
	endToEndTest(t, prpc.FormatText)
}

func endToEndTest(t *testing.T, responseFormat prpc.Format) {
	ftt.Run(`A client/server for the Greeter service`, t, func(t *ftt.Test) {
		ctx := gologger.StdConfig.Use(context.Background())
		svc := service{
			sleep: func() time.Duration { return time.Millisecond },
		}
		ts, prpcC, client := newTestClient(ctx, &svc, &prpc.Options{
			ResponseFormat: responseFormat,
		})
		defer ts.Close()

		ts.MaxRequestSize = 2 * 1024 * 1024

		t.Run(`Can round-trip a hello message`, func(t *ftt.Test) {
			svc.R = &testpb.HelloReply{Message: "sup"}

			resp, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, should.Match(svc.R))
		})

		t.Run(`Can round-trip an empty message`, func(t *ftt.Test) {
			svc.R = &testpb.HelloReply{}

			resp, err := client.SayHello(ctx, &testpb.HelloRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, should.Match(svc.R))
		})

		t.Run(`Respects response size limits`, func(t *ftt.Test) {
			var retried atomic.Bool

			prpcC.Options.Retry = func() retry.Iterator {
				return retry.NewIterator(func(context.Context, error) time.Duration {
					retried.Store(true)
					return retry.Stop
				})
			}
			prpcC.MaxResponseSize = 123

			svc.R = &testpb.HelloReply{Message: strings.Repeat("z", 124)}

			_, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Unavailable))
			assert.Loosely(t, err, should.ErrLike("exceeds the client limit 123"))
			assert.Loosely(t, prpc.ProtocolErrorDetails(err).GetResponseTooBig(), should.NotBeNil)

			// Doesn't trigger a retry.
			assert.Loosely(t, retried.Load(), should.BeFalse)
		})

		t.Run(`Can send a giant message with compression`, func(t *ftt.Test) {
			svc.R = &testpb.HelloReply{Message: "sup"}

			msg := make([]byte, 512*1024)
			_, err := rand.Read(msg)
			assert.Loosely(t, err, should.BeNil)

			resp, err := client.SayHello(ctx, &testpb.HelloRequest{Name: hex.EncodeToString(msg)})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, should.Match(svc.R))
		})

		t.Run(`Can receive a giant message with compression`, func(t *ftt.Test) {
			msg := make([]byte, 512*1024)
			_, err := rand.Read(msg)
			assert.Loosely(t, err, should.BeNil)

			svc.R = &testpb.HelloReply{Message: hex.EncodeToString(msg)}

			resp, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "hi"})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, should.Match(svc.R))
		})

		t.Run(`Rejects mega giant uncompressed request`, func(t *ftt.Test) {
			prpcC.EnableRequestCompression = false
			_, err := client.SayHello(ctx, &testpb.HelloRequest{
				Name: strings.Repeat("z", 2*1024*1024),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Unavailable))
			assert.Loosely(t, err, should.ErrLike("reading the request: the request size exceeds the server limit"))
		})

		t.Run(`Rejects mega-giant compressed request`, func(t *ftt.Test) {
			_, err := client.SayHello(ctx, &testpb.HelloRequest{
				Name: strings.Repeat("z", 2*1024*1024),
			})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.Unavailable))
			assert.Loosely(t, err, should.ErrLike("decompressing the request: the decompressed request size exceeds the server limit"))
		})

		t.Run(`Can round-trip status details`, func(t *ftt.Test) {
			detail := &errdetails.DebugInfo{Detail: "x"}

			s := status.New(codes.AlreadyExists, "already exists")
			s, err := s.WithDetails(detail)
			assert.Loosely(t, err, should.BeNil)
			svc.err = s.Err()

			_, err = client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
			details := status.Convert(err).Details()
			assert.Loosely(t, details, should.Match([]any{detail}))
		})

		t.Run(`Can round-trip plural status details`, func(t *ftt.Test) {
			detail1 := &errdetails.DebugInfo{Detail: "x"}
			detail2 := &errdetails.DebugInfo{Detail: "y"}

			s := status.New(codes.AlreadyExists, "already exists")
			s, err := s.WithDetails(detail1)
			assert.Loosely(t, err, should.BeNil)
			s, err = s.WithDetails(detail2)
			assert.Loosely(t, err, should.BeNil)
			svc.err = s.Err()

			_, err = client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
			details := status.Convert(err).Details()
			assert.Loosely(t, details, should.Match([]any{detail1, detail2}))
		})

		t.Run(`Can handle non-trivial metadata`, func(t *ftt.Test) {
			md := metadata.New(nil)
			md.Append("MultiVAL-KEY", "val 1", "val 2")
			md.Append("binary-BIN", string([]byte{0, 1, 2, 3}))

			svc.R = &testpb.HelloReply{Message: "sup"}
			svc.outgoingMD = md

			var respMD metadata.MD

			ctx = metadata.NewOutgoingContext(ctx, md)
			resp, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"}, grpc.Header(&respMD))
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
			assert.Loosely(t, resp, should.Match(svc.R))

			assert.Loosely(t, svc.getIncomingMD(), should.Match(metadata.MD{
				":authority":   {ts.Host},
				"binary-bin":   {string([]byte{0, 1, 2, 3})},
				"cookie":       {"cookie_1=value_1; cookie_2=value_2"},
				"host":         {strings.TrimPrefix(ts.HTTP.URL, "http://")},
				"multival-key": {"val 1", "val 2"},
				"user-agent":   {prpc.DefaultUserAgent},
			}))

			assert.Loosely(t, respMD, should.Match(metadata.MD{
				"binary-bin":   {string([]byte{0, 1, 2, 3})},
				"multival-key": {"val 1", "val 2"},
			}))
		})

		t.Run(`Populates peer`, func(t *ftt.Test) {
			svc.R = &testpb.HelloReply{Message: "sup"}
			_, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))

			peer := svc.getIncomingPeer()
			assert.Loosely(t, peer, should.NotBeNil)
			assert.Loosely(t, peer.Addr.String(), should.HavePrefix("127.0.0.1:"))
		})
	})
}

func TestJSONFieldMaskWithoutHack(t *testing.T) {
	t.Parallel()

	// Doesn't support advanced masks.
	testJSONFieldMask(t, false, func(srv *prpc.Server) {
		srv.EnableNonStandardFieldMasks = false
	})
}

func TestJSONFieldMaskWithHack(t *testing.T) {
	t.Parallel()

	// Supports advanced masks.
	testJSONFieldMask(t, true, func(srv *prpc.Server) {
		srv.EnableNonStandardFieldMasks = true
	})
}

func testJSONFieldMask(t *testing.T, testAdvanced bool, cfg func(srv *prpc.Server)) {
	ctx := gologger.StdConfig.Use(context.Background())
	svc := service{R: &testpb.HelloReply{Message: "sup"}}

	ts := prpctest.Server{}
	testpb.RegisterGreeterServer(&ts, &svc)

	if cfg != nil {
		cfg(&ts.Server)
	}

	ts.Start(ctx)
	defer ts.Close()

	// Get an HTTP client since we specifically want to test concrete JSON
	// serialization formats for backward compatibility and using a Go pRPC
	// client makes this hard since we will need to manipulate it to produce
	// necessary serialization first.
	cl, err := ts.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	httpC := cl.C

	jsonCall := func(body string) error {
		req, err := http.NewRequest(
			"POST",
			fmt.Sprintf("http://%s/prpc/prpc.Greeter/SayHello", ts.Host),
			strings.NewReader(body),
		)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := httpC.Do(req)
		if err != nil {
			return err
		}
		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("failed with code %d: %s", resp.StatusCode, respBody)
		}
		return nil
	}

	t.Run(`Simple mask as str`, func(t *testing.T) {
		err := jsonCall(`{"fields": "a,a.b.c"}`)
		assert.That(t, err, should.ErrLike(nil))
		assert.That(t, svc.getIncomingFields(), should.Match([]string{
			"a", "a.b.c",
		}))
	})

	if testAdvanced {
		t.Run(`Advanced mask as str`, func(t *testing.T) {
			err := jsonCall(`{"fields": "a.*.b,a.0.c"}`)
			assert.That(t, err, should.ErrLike(nil))
			assert.That(t, svc.getIncomingFields(), should.Match([]string{
				"a.*.b", "a.0.c",
			}))
		})
	}
}

func TestTimeouts(t *testing.T) {
	t.Parallel()

	ftt.Run(`A client/server for the Greeter service`, t, func(t *ftt.Test) {
		ctx := gologger.StdConfig.Use(context.Background())
		svc := service{R: &testpb.HelloReply{Message: "sup"}}
		ts, _, client := newTestClient(ctx, &svc, &prpc.Options{
			Retry: func() retry.Iterator {
				return &retry.ExponentialBackoff{
					Limited: retry.Limited{
						Delay:   time.Millisecond,
						Retries: 3,
					},
				}
			},
			PerRPCTimeout: time.Second,
		})
		defer ts.Close()

		t.Run(`Gives up after N retries`, func(t *ftt.Test) {
			svc.sleep = func() time.Duration {
				return 60 * time.Second // much larger than the per-RPC timeout
			}

			_, err := client.SayHello(ctx, &testpb.HelloRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.DeadlineExceeded))
		})

		t.Run(`Succeeds after N retries`, func(t *ftt.Test) {
			attempt := 0
			svc.sleep = func() time.Duration {
				attempt += 1
				if attempt > 2 {
					return 0
				}
				return 60 * time.Second
			}

			_, err := client.SayHello(ctx, &testpb.HelloRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
		})

		t.Run(`Gives up on overall timeout`, func(t *ftt.Test) {
			svc.sleep = func() time.Duration {
				return 60 * time.Second // much larger than the per-RPC timeout
			}

			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			_, err := client.SayHello(ctx, &testpb.HelloRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.DeadlineExceeded))
		})

		t.Run(`Respected DEADLINE_EXCEEDED response code`, func(t *ftt.Test) {
			svc.err = status.Errorf(codes.DeadlineExceeded, "internal deadline exceeded")

			_, err := client.SayHello(ctx, &testpb.HelloRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.DeadlineExceeded))
		})
	})
}

func TestVerySmallTimeouts(t *testing.T) {
	t.Parallel()

	ftt.Run(`A client/server for the Greeter service`, t, func(t *ftt.Test) {
		ctx := gologger.StdConfig.Use(context.Background())
		svc := service{}
		ts, _, client := newTestClient(ctx, &svc, &prpc.Options{
			PerRPCTimeout: 10 * time.Millisecond,
		})
		defer ts.Close()

		// There should be either no error or DeadlineExceeded error (depending on
		// how speedy is the test runner). There should never be any other errors.
		// This test is inherently non-deterministic since it depends on various
		// places in net/http network guts that can abort the connection.

		t.Run(`Round-trip a hello message`, func(t *ftt.Test) {
			svc.R = &testpb.HelloReply{Message: "sup"}

			_, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
			if err != nil {
				assert.Loosely(t, err, grpccode.ShouldBe(codes.DeadlineExceeded))
			}
		})

		t.Run(`Send a giant message with compression`, func(t *ftt.Test) {
			svc.R = &testpb.HelloReply{Message: "sup"}

			msg := make([]byte, 1024*1024)
			_, err := rand.Read(msg)
			assert.Loosely(t, err, should.BeNil)

			_, err = client.SayHello(ctx, &testpb.HelloRequest{Name: hex.EncodeToString(msg)})
			if err != nil {
				assert.Loosely(t, err, grpccode.ShouldBe(codes.DeadlineExceeded))
			}
		})

		t.Run(`Receive a giant message with compression`, func(t *ftt.Test) {
			msg := make([]byte, 1024*1024)
			_, err := rand.Read(msg)
			assert.Loosely(t, err, should.BeNil)

			svc.R = &testpb.HelloReply{Message: hex.EncodeToString(msg)}

			_, err = client.SayHello(ctx, &testpb.HelloRequest{Name: "hi"})
			if err != nil {
				assert.Loosely(t, err, grpccode.ShouldBe(codes.DeadlineExceeded))
			}
		})
	})
}

func TestMaxLimits(t *testing.T) {
	t.Parallel()

	ctx := gologger.StdConfig.Use(context.Background())
	svc := service{}
	ts, tc, client := newTestClient(ctx, &svc, nil)
	defer ts.Close()

	ts.MaxRequestSize = math.MaxInt
	tc.MaxResponseSize = math.MaxInt

	// Doesn't blow up.
	svc.R = &testpb.HelloReply{Message: "sup"}
	resp, err := client.SayHello(ctx, &testpb.HelloRequest{Name: "round-trip"})
	assert.Loosely(t, err, grpccode.ShouldBe(codes.OK))
	assert.Loosely(t, resp, should.Match(svc.R))
}
