// Copyright 2026 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generators

import (
	"cmp"
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/cipkg/core"
)

// Ref is a generator that creates an alias action referencing Target's action
// while preserving Target's Derivation and DerivationID.
type Ref struct {
	Name     string
	Metadata *core.Action_Metadata

	Target Generator
}

func (r *Ref) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	if r.Target == nil {
		return nil, fmt.Errorf("ref generator target must not be nil")
	}

	target, err := r.Target.Generate(ctx, plats)
	if err != nil {
		return nil, err
	}

	var metadata *core.Action_Metadata
	if r.Metadata != nil {
		metadata = proto.CloneOf(r.Metadata)
	}
	if tm := target.GetMetadata(); tm != nil && (len(tm.RuntimeDeps) > 0 || tm.Untrusted) {
		if metadata == nil {
			metadata = &core.Action_Metadata{}
		}
		metadata.RuntimeDeps = append(metadata.RuntimeDeps, tm.RuntimeDeps...)
		metadata.Untrusted = metadata.Untrusted || tm.Untrusted
	}

	return &core.Action{
		Name:     cmp.Or(r.Name, target.Name),
		Metadata: metadata,
		Deps:     []*core.Action{target},
		Spec: &core.Action_Ref{
			Ref: &core.ActionRef{},
		},
	}, nil
}
