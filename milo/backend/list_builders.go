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

package backend

import (
	"context"

	"go.chromium.org/luci/grpc/appstatus"
	milopb "go.chromium.org/luci/milo/api/service/v1"
	"google.golang.org/grpc/codes"
)

// ListBuilders implements milopb.MiloInternal service
func (s *MiloInternalService) ListBuilders(ctx context.Context, req *milopb.ListBuildersRequest) (_ *milopb.ListBuildersResponse, err error) {
	defer func() { err = appstatus.GRPCifyAndLog(ctx, err) }()

	return nil, appstatus.Error(codes.Unimplemented, "unimplemented")
}
