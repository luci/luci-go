// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/luci/luci-go/common/proto/google"
	milo "github.com/luci/luci-go/milo/api/proto"
	"golang.org/x/net/context"
)

type BuildbotService struct{}

var errNotFoundGRPC = grpc.Errorf(codes.NotFound, "Master Not Found")

func (s *BuildbotService) GetCompressedMasterJSON(
	c context.Context, req *milo.MasterRequest) (*milo.CompressedMasterJSON, error) {

	if req.Name == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "No master specified")
	}

	entry, err := getMasterEntry(c, req.Name)
	switch {
	case err == errMasterNotFound:
		return nil, errNotFoundGRPC

	case err != nil:
		return nil, err
	}

	return &milo.CompressedMasterJSON{
		Internal: entry.Internal,
		Modified: &google.Timestamp{
			Seconds: entry.Modified.Unix(),
			Nanos:   int32(entry.Modified.Nanosecond()),
		},
		Data: entry.Data,
	}, nil
}
