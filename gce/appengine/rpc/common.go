// Copyright 2018 The LUCI Authors.
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

package rpc

import (
	"context"
	"strings"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/grpc/grpcutil"
)

const (
	admins  = "gce-provider-administrators"
	readers = "gce-provider-readers"
	writers = "gce-provider-writers"
)

// isReadOnly returns whether the given method name is for a read-only method.
func isReadOnly(methodName string) bool {
	return strings.HasPrefix(methodName, "Get") || strings.HasPrefix(methodName, "List")
}

// gRPCifyAndLogErr ensures any error being returned is a gRPC error, logging Internal and Unknown errors.
func gRPCifyAndLogErr(c context.Context, methodName string, rsp proto.Message, err error) error {
	return grpcutil.GRPCifyAndLogErr(c, err)
}
