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

package backend

import (
	"context"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/milo/git"
	"go.chromium.org/luci/server/auth"
)

// MiloInternalService implements milopb.MiloInternal
type MiloInternalService struct {
	// GetGitClient returns a git client for the given context.
	GetGitClient func(c context.Context) (git.Client, error)

	// GetBuildersClient returns a buildbucket builders service for the given
	// context.
	GetBuildersClient func(c context.Context, as auth.RPCAuthorityKind) (buildbucketpb.BuildersClient, error)
}
