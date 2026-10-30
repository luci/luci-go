// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"context"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/config"

	"go.chromium.org/luci/server/module"
)

type noopStore[T proto.Message] struct {
	lastPut T
}

func (*noopStore[T]) ModuleDependencies() []module.Dependency {
	return []module.Dependency{module.OptionalDependency(ModuleName)}
}

func (*noopStore[T]) LoadMeta(ctx context.Context, pipelineID string) (map[config.Set]Meta, error) {
	return nil, nil
}

func (s *noopStore[T]) Put(ctx context.Context, pipelineID string, cs config.Set, m Meta, out T) error {
	s.lastPut = out
	return nil
}

func (*noopStore[T]) Delete(ctx context.Context, pipelineID string, sets []config.Set) error {
	return nil
}
