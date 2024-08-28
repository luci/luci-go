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

package actions

import (
	"errors"
	"fmt"
	"regexp"
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/cipkg/core"
)

// Package represents a high-level package including:
// - A storage handler for the package.
// - The transformed derivation and its unique id.
// - All its dependencies.
// - The action responsible for the package's content.
// All dependencies will be available duing build time (e.g for testing) but
// only runtime dependencies will be ensured to be available at runtime.
// Potentially we can cache or reuse package to avoid transform the same action
// multiple times if performance is to be considered.
type Package struct {
	Handler core.PackageHandler

	DerivationID string
	Derivation   *core.Derivation

	BuildDependencies   []Package
	RuntimeDependencies []Package

	ActionID string
	Action   *core.Action
}

var (
	ErrUnsupportedAction = errors.New("unsupported action spec")
)

// Transformer is the function transforms an action specification to a
// derivation.
type Transformer[M proto.Message] func(M, []Package) (*core.Derivation, error)

// ActionProcessor processes and transforms actions into packages.
type ActionProcessor struct {
	transformers map[protoreflect.FullName]Transformer[proto.Message]

	mu     sync.Mutex
	sealed bool
}

func NewActionProcessor() *ActionProcessor {
	ap := &ActionProcessor{
		transformers: make(map[protoreflect.FullName]Transformer[proto.Message]),
	}
	MustSetTransformer[*core.ActionCommand](ap, ActionCommandTransformer)
	MustSetTransformer[*core.ActionURLFetch](ap, ActionURLFetchTransformer)
	MustSetTransformer[*core.ActionFilesCopy](ap, ActionFilesCopyTransformer)
	MustSetTransformer[*core.ActionCIPDExport](ap, ActionCIPDExportTransformer)
	return ap
}

var (
	packageNameRe = regexp.MustCompile(`^[a-z_]+[a-z0-9_\.]*$`)

	ErrTransformerExisted    = errors.New("transformer for the message type already existed")
	ErrActionProcessorSealed = errors.New("transformer can't be set after processor being used")
)

// SetTransformer set the transformer for the action specification M.
func SetTransformer[M proto.Message](ap *ActionProcessor, tf Transformer[M]) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.sealed {
		return ErrActionProcessorSealed
	}

	var m M
	name := proto.MessageName(m)
	if _, ok := ap.transformers[name]; ok {
		return ErrTransformerExisted
	}
	ap.transformers[name] = func(m proto.Message, deps []Package) (*core.Derivation, error) {
		return tf(m.(M), deps)
	}
	return nil
}

// MustSetTransformer set the transformer for the action specification M and
// panic if any error happened.
func MustSetTransformer[M proto.Message](ap *ActionProcessor, tf Transformer[M]) {
	if err := SetTransformer[M](ap, tf); err != nil {
		panic(err)
	}
}

// Process transforms a given *core.Action into a self-contained Package, which
// includes all its dependencies also converted from *core.Action into Package.
func (ap *ActionProcessor) Process(buildPlat string, pm core.PackageManager, a *core.Action) (Package, error) {
	ap.mu.Lock()
	if !ap.sealed {
		ap.sealed = true
	}
	ap.mu.Unlock()

	if !packageNameRe.MatchString(a.Name) {
		return Package{}, fmt.Errorf("action name must conform %s: %s", packageNameRe, a)
	}

	aid, err := core.GetActionID(a)
	if err != nil {
		return Package{}, err
	}
	pkg := Package{
		ActionID: aid,
		Action:   a,
	}

	// Recursivedly process all dependencies
	for _, d := range a.Deps {
		dpkg, err := ap.Process(buildPlat, pm, d)
		if err != nil {
			return Package{}, err
		}

		pkg.BuildDependencies = append(pkg.BuildDependencies, dpkg)
	}
	// Runtime dependencies are not required during the execution of the action.
	// They won't be included in the derivation's input.
	if a.Metadata != nil {
		for _, d := range a.Metadata.RuntimeDeps {
			dpkg, err := ap.Process(buildPlat, pm, d)
			if err != nil {
				return Package{}, err
			}

			pkg.RuntimeDependencies = append(pkg.RuntimeDependencies, dpkg)
		}
	}

	// Parse spec message
	var spec proto.Message

	if s, ok := a.Spec.(*core.Action_Extension); ok {
		if spec, err = s.Extension.UnmarshalNew(); err != nil {
			return Package{}, fmt.Errorf("%s: %w", a.Spec, err)
		}
	} else {
		rm := a.ProtoReflect()
		fds := rm.Descriptor().Oneofs().ByName("spec")
		fd := rm.WhichOneof(fds)
		if fd == nil {
			return Package{}, fmt.Errorf("%s: no action spec available: %w", a.Spec, ErrUnsupportedAction)
		}
		spec = rm.Get(fd).Message().Interface()
	}

	// Transform spec message
	tr := ap.transformers[proto.MessageName(spec)]
	if tr == nil {
		return Package{}, ErrUnsupportedAction
	}
	drv, err := tr(spec, pkg.BuildDependencies)
	if err != nil {
		return Package{}, fmt.Errorf("%s: %w", a.Spec, err)
	}

	// Update common derivation fields
	drv.Name = a.Name
	drv.Platform = buildPlat
	for _, d := range pkg.BuildDependencies {
		drv.Inputs = append(drv.Inputs, d.DerivationID)
	}
	pkg.Derivation = drv

	// Calculate DerivationID
	drvID, err := core.GetDerivationID(pkg.Derivation)
	if err != nil {
		return Package{}, err
	}
	pkg.DerivationID = drvID
	pkg.Handler = pm.Get(drvID)

	return pkg, nil
}
