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

package buildsource

import (
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/milo/internal/model"
)

// BuilderID is the universal ID of a builder, and has the form:
//
//	buildbucket/bucket/builder
type BuilderID string

// Split breaks the BuilderID into pieces.
//   - backend is always 'buildbucket'
//   - backendGroup is either the bucket or builder group
//   - builderName is the builder name.
//
// Returns an error if the BuilderID is malformed (wrong # slashes) or if any of
// the pieces are empty.
func (b BuilderID) Split() (backend, backendGroup, builderName string, err error) {
	toks := strings.SplitN(string(b), "/", 3)
	if len(toks) != 3 {
		err =
			grpcutil.InvalidArgumentTag.Apply(errors.Fmt("bad BuilderID: not enough tokens: %q", b))

		return
	}
	backend, backendGroup, builderName = toks[0], toks[1], toks[2]
	switch {
	case backend != "buildbucket":
		err =
			grpcutil.InvalidArgumentTag.Apply(errors.Fmt("bad BuilderID: unknown backend %q", backend))

	case backendGroup == "":
		err = grpcutil.InvalidArgumentTag.Apply(errors.New("bad BuilderID: empty backendGroup"))
	case builderName == "":
		err = grpcutil.InvalidArgumentTag.Apply(errors.New("bad BuilderID: empty builderName"))
	}
	return
}

// SelfLink returns LUCI URL of the builder.
func (b BuilderID) SelfLink(project string) string {
	return model.BuilderIDLink(string(b), project)
}
