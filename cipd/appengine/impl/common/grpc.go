// Copyright 2017 The LUCI Authors.
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

package common

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
)

// GRPCifyAndLogErr converts an annotated luci error to a gRPC error and logs
// internal details (including stack trace) for errors with Internal or Unknown
// codes.
//
// If err is already gRPC error (or nil), it is silently passed through, even
// if it is Internal. There's nothing interesting to log in this case.
func GRPCifyAndLogErr(c context.Context, err error) error {
	if err == nil {
		return nil
	}
	if _, yep := status.FromError(err); yep {
		return err
	}
	grpcErr := grpcutil.ToGRPCErr(err)
	code := grpc.Code(grpcErr)
	if code == codes.Internal || code == codes.Unknown {
		errors.Log(c, err)
	}
	return grpcErr
}
