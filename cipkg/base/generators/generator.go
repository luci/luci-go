// Copyright 2023 The LUCI Authors.
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
	"context"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipkg/core"
)

// Generator is the interface for generating actions.
type Generator interface {
	Generate(ctx context.Context, plats Platforms) (*core.Action, error)
}

var (
	ErrUnknowDependencyType = errors.New("unknown dependency type")
)

// DependencyType includes the different dependency types are used to calculate
// dependency's cross-compile platform from the dependent's.
type DependencyType int

func (t DependencyType) String() string {
	switch t {
	case DepsBuildBuild:
		return "depsBuildBuild"
	case DepsBuildHost:
		return "depsBuildHost"
	case DepsBuildTarget:
		return "depsBuildTarget"
	case DepsHostHost:
		return "depsHostHost"
	case DepsHostTarget:
		return "depsHostTarget"
	case DepsTargetTarget:
		return "depsTargetTarget"
	default:
		return "depsUnknown"
	}
}

const (
	DepsUnknown DependencyType = iota
	DepsBuildBuild
	DepsBuildHost
	DepsBuildTarget
	DepsHostHost
	DepsHostTarget
	DepsTargetTarget
	DepsMaxNum
)

// Dependency provide function for generating the corresponding action with
// platform from the dependency type for cross-compilation.
type Dependency struct {
	Generator Generator
	Type      DependencyType
	Runtime   bool
}

// Generate the corresponding action with platform from the dependency type for
// cross-compilation.
func (dep *Dependency) Generate(ctx context.Context, plats Platforms) (*core.Action, error) {
	var depPlats Platforms
	switch dep.Type {
	case DepsBuildBuild:
		depPlats = Platforms{plats.Build, plats.Build, plats.Build}
	case DepsBuildHost:
		depPlats = Platforms{plats.Build, plats.Build, plats.Host}
	case DepsBuildTarget:
		depPlats = Platforms{plats.Build, plats.Build, plats.Target}
	case DepsHostHost:
		depPlats = Platforms{plats.Build, plats.Host, plats.Host}
	case DepsHostTarget:
		depPlats = Platforms{plats.Build, plats.Host, plats.Target}
	case DepsTargetTarget:
		depPlats = Platforms{plats.Build, plats.Target, plats.Target}
	default:
		return nil, ErrUnknowDependencyType
	}
	return dep.Generator.Generate(ctx, depPlats)
}
