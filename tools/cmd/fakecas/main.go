// Copyright 2021 The LUCI Authors.
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

package main

import (
	"context"
	"log"
	"net"

	"github.com/bazelbuild/remote-apis-sdks/go/pkg/fakes"
	regrpc "github.com/bazelbuild/remote-apis/build/bazel/remote/execution/v2"
	bsgrpc "google.golang.org/genproto/googleapis/bytestream"
	"google.golang.org/grpc"

	"go.chromium.org/luci/common/system/signals"
)

const port = ":9000"

func main() {
	s := grpc.NewServer()
	cas := fakes.NewCAS()
	ex := &exec{}
	bsgrpc.RegisterByteStreamServer(s, cas)
	regrpc.RegisterContentAddressableStorageServer(s, cas)
	regrpc.RegisterCapabilitiesServer(s, ex)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v\n", err)
	}
	log.Printf("listening tcp port %s\n", port)

	defer signals.HandleInterrupt(func() {
		log.Println("shutting down fake CAS gRPC server...")
		s.GracefulStop()
	})()

	log.Println("starting CAS fake server...")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve fake CAS gRPC server: %v\n", err)
	}
}

// exec implements only GetCapabilities for CAS client.
type exec struct{}

// GetCapabilities returns the fake capabilities.
func (c *exec) GetCapabilities(ctx context.Context, req *regrpc.GetCapabilitiesRequest) (res *regrpc.ServerCapabilities, err error) {
	return &regrpc.ServerCapabilities{}, nil
}
