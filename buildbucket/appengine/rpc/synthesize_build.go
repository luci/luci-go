// Copyright 2022 The LUCI Authors.
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
	"errors"

	pb "go.chromium.org/luci/buildbucket/proto"
)

// SynthesizeBuild handles a request to synthesize a build. Implements pb.BuildsServer.
func (*Builds) SynthesizeBuild(ctx context.Context, req *pb.SynthesizeBuildRequest) (*pb.Build, error) {
	return nil, errors.New("not implemented")
}
