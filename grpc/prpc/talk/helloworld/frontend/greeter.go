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

package helloworld

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/grpc/prpc/talk/helloworld/proto"
)

type greeterService struct{}

func (s *greeterService) SayHello(c context.Context, req *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	if req.Name == "" {
		return nil, status.Errorf(codes.InvalidArgument, "Name unspecified")
	}

	return &helloworld.HelloReply{
		Message: "Hello " + req.Name,
	}, nil
}

var _ helloworld.GreeterServer = (*greeterService)(nil)
