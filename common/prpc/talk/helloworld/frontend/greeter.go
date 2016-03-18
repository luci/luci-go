// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package helloworld

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/prpc/talk/helloworld/proto"
)

type greeterService struct{}

func (s *greeterService) SayHello(c context.Context, req *helloworld.HelloRequest) (*helloworld.HelloReply, error) {
	if req.Name == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "Name unspecified")
	}

	return &helloworld.HelloReply{
		Message: "Hello " + req.Name,
	}, nil
}

var _ helloworld.GreeterServer = (*greeterService)(nil)
