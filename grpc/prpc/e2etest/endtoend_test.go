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
	"strings"
	"sync"
	"testing"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/testing/prpctest"
	"go.chromium.org/luci/grpc/prpc"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type service struct {
	R          *HelloReply
	err        error
	outgoingMD metadata.MD

	m          sync.Mutex
	incomingMD metadata.MD
}

func (s *service) Greet(c context.Context, req *HelloRequest) (*HelloReply, error) {
	md, _ := metadata.FromIncomingContext(c)
	s.m.Lock()
	s.incomingMD = md.Copy()
	s.m.Unlock()

	if s.outgoingMD != nil {
		if err := prpc.SetHeader(c, s.outgoingMD); err != nil {
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

func TestEndToEnd(t *testing.T) {
	Convey(`A client/server for the Greet service`, t, func() {
		c := context.Background()
		svc := service{}

		// Create a client/server for Greet service.
		ts := prpctest.Server{}
		RegisterHelloServer(&ts, &svc)
		ts.Start(c)
		defer ts.Close()

		prpcClient, err := ts.NewClient()
		if err != nil {
			panic(err)
		}

		// TODO(crbug.com/1336810): Request compression is broken now. Uncommenting
		// this will break the test.
		//
		// prpcClient.EnableRequestCompression = true

		client := NewHelloClient(prpcClient)

		Convey(`Can round-trip a hello message`, func() {
			svc.R = &HelloReply{Message: "sup"}

			resp, err := client.Greet(c, &HelloRequest{Name: "round-trip"})
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)
		})

		Convey(`Can send a giant message with compression`, func() {
			svc.R = &HelloReply{Message: "sup"}

			msg := make([]byte, 10*1024*1024)
			_, err := rand.Read(msg)
			So(err, ShouldBeNil)

			resp, err := client.Greet(c, &HelloRequest{Name: hex.EncodeToString(msg)})
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)
		})

		Convey(`Can round-trip status details`, func() {
			detail := &errdetails.DebugInfo{Detail: "x"}

			s := status.New(codes.Internal, "internal")
			s, err := s.WithDetails(detail)
			So(err, ShouldBeNil)
			svc.err = s.Err()

			_, err = client.Greet(c, &HelloRequest{Name: "round-trip"})
			details := status.Convert(err).Details()
			So(details, ShouldResembleProto, []proto.Message{detail})
		})

		Convey(`Can handle non-trivial metadata`, func() {
			md := metadata.New(nil)
			md.Append("MultiVAL-KEY", "val 1", "val 2")
			md.Append("binary-BIN", string([]byte{0, 1, 2, 3}))

			svc.R = &HelloReply{Message: "sup"}
			svc.outgoingMD = md

			var respMD metadata.MD

			c = metadata.NewOutgoingContext(c, md)
			resp, err := client.Greet(c, &HelloRequest{Name: "round-trip"}, grpc.Header(&respMD))
			So(err, ShouldBeRPCOK)
			So(resp, ShouldResembleProto, svc.R)

			So(svc.getIncomingMD(), ShouldResemble, metadata.MD{
				"binary-bin":   {string([]byte{0, 1, 2, 3})},
				"host":         {strings.TrimPrefix(ts.HTTP.URL, "http://")},
				"multival-key": {"val 1", "val 2"},
				"user-agent":   {prpc.DefaultUserAgent},
			})

			So(respMD, ShouldResemble, metadata.MD{
				"binary-bin":   {string([]byte{0, 1, 2, 3})},
				"multival-key": {"val 1", "val 2"},
			})
		})
	})
}
