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

package certconfig

import (
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
)

// ListCAsRPC implements CertificateAuthorities.ListCAs RPC method.
type ListCAsRPC struct {
}

// ListCAs returns a list of Common Names of registered CAs.
func (r *ListCAsRPC) ListCAs(c context.Context, _ *empty.Empty) (*admin.ListCAsResponse, error) {
	names, err := ListCAs(c)
	if err != nil {
		return nil, grpc.Errorf(codes.Internal, "transient datastore error - %s", err)
	}
	return &admin.ListCAsResponse{Cn: names}, nil
}
