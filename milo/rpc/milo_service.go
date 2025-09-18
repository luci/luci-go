// Copyright 2020 The LUCI Authors.
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

	bbgrpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/server/auth"

	configpb "go.chromium.org/luci/milo/proto/config"
)

// MiloInternalService implements milopb.MiloInternal
type MiloInternalService struct {
	// GetSettings returns the current setting for milo.
	GetSettings func(c context.Context) (*configpb.Settings, error)

	// GetGitilesClient returns a git client for the given context.
	GetGitilesClient func(c context.Context, host string, as auth.RPCAuthorityKind) (gitiles.GitilesClient, error)

	// GetBuildersClient returns a buildbucket builders service for the given
	// context.
	GetBuildersClient func(c context.Context, host string, as auth.RPCAuthorityKind) (bbgrpcpb.BuildersClient, error)
}
