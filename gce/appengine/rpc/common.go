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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/gce/vmtoken"
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

// readOnlyAuthPrelude ensures the user is authorized to use read API methods.
// Always returns permission denied for write API methods.
func readOnlyAuthPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	if !isReadOnly(methodName) {
		return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
	}
	switch is, err := auth.IsMember(c, admins, writers, readers); {
	case err != nil:
		return c, err
	case is:
		logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
		return c, nil
	}
	return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
}

// isVMAccessible returns whether the given method name may be accessed by VMs.
// Methods here must perform additional authorization checks if vmtoken.Has
// returns true.
func isVMAccessible(methodName string) bool {
	return methodName == "Get"
}

// vmAccessPrelude ensures the user is authorized to use the API or has
// presented a valid GCE VM token and is attempting to use VM-accessible API.
// Users of this prelude must perform additional authorization checks in methods
// accepted by isVMAccessible.
func vmAccessPrelude(c context.Context, methodName string, req proto.Message) (context.Context, error) {
	groups := []string{admins, writers}
	if isReadOnly(methodName) {
		groups = append(groups, readers)
	}
	switch is, err := auth.IsMember(c, groups...); {
	case err != nil:
		return c, err
	case is:
		logging.Debugf(c, "%s called %q:\n%s", auth.CurrentIdentity(c), methodName, req)
		// Remove the VM token to avoid restricting the user's broad access.
		return vmtoken.Clear(c), nil
	case isVMAccessible(methodName) && vmtoken.Has(c):
		logging.Debugf(c, "%s called %q:\n%s", vmtoken.CurrentIdentity(c), methodName, req)
		return c, nil
	}
	return c, status.Errorf(codes.PermissionDenied, "unauthorized user")
}

// gRPCifyAndLogErr ensures any error being returned is a gRPC error, logging Internal and Unknown errors.
func gRPCifyAndLogErr(c context.Context, methodName string, rsp proto.Message, err error) error {
	return grpcutil.GRPCifyAndLogErr(c, err)
}
