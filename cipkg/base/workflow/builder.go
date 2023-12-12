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

package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type PreExecuteHook func(ctx context.Context, pkg actions.Package) error

// ExecutionConfig includes all configs for Executor.
type ExecutionConfig struct {
	OutputDir  string
	WorkingDir string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// Executor is the function interface for executing the provided derivation.
// This can be subject to the platform or using sandbox for isolation.
type Executor func(ctx context.Context, cfg *ExecutionConfig, drv *core.Derivation) error

// Execute is the default Executor which runs the command presented by the
// derivation.
func Execute(ctx context.Context, cfg *ExecutionConfig, drv *core.Derivation) error {
	cmd := exec.CommandContext(ctx, drv.Args[0], drv.Args[1:]...)
	cmd.Path = drv.Args[0]
	cmd.Dir = cfg.WorkingDir
	cmd.Stdin = cfg.Stdin
	cmd.Stdout = cfg.Stdout
	cmd.Stderr = cfg.Stderr
	cmd.Env = slices.Clone(drv.Env)

	// Set out last to make sure it won't be overridden.
	cmd.Env = append(cmd.Env, "out="+cfg.OutputDir)
	return cmd.Run()
}

var (
	ErrExecutionPlanExecuted = errors.New("execution plan has been executed")
)

// ExecutionPlan is the plan for packages to be built by running executor.
// It will ensure all packages' dependencies added into the plan and will
// become available in storage when the package is built.
// See also: Builder includes a standard workflow for using ExecutionPlan.
type ExecutionPlan struct {
	// newPkgs are new packages planned to be built. The build order shouldn't
	// be relied on and packages may be executed in parallel.
	newPkgs []actions.Package

	// availables are All available packages will be referenced. After a package
	// is built, it will also be added to availables.
	availables []actions.Package

	// added are set of derivation ids that added to the plan, regardless of
	// their availabilities.
	added stringset.Set

	executed bool
}

// NewExecutionPlan generates an execution plan for building packages to make
// all of them and their dependencies available. A preExecFn can be provided to
// e.g. fetch package from local or remote cache before the package being added
// to the plan.
func NewExecutionPlan(ctx context.Context) *ExecutionPlan {
	return &ExecutionPlan{
		added: stringset.New(10),
	}
}

// Add adds a package and all its dependencies to the execution plan. If
// preExecFn is not empty, it will be executed to possibly make the package
// available before execution.
func (p *ExecutionPlan) Add(ctx context.Context, pkg actions.Package, preExecFn PreExecuteHook) error {
	if p.executed {
		return ErrExecutionPlanExecuted
	}
	if _, ok := p.added[pkg.DerivationID]; ok {
		return nil
	}

	// Add runtime dependencies in any case.
	for _, d := range pkg.RuntimeDependencies {
		if err := p.Add(ctx, d, preExecFn); err != nil {
			return err
		}
	}

	switch err := pkg.Handler.IncRef(); {
	case errors.Is(err, core.ErrPackageNotExist):
		if preExecFn != nil {
			if err := preExecFn(ctx, pkg); err != nil {
				return fmt.Errorf("failed to run preExecute hook for the package: %s: %w", pkg.DerivationID, err)
			}

			// Add the package again without PreExecuteHook. If preExecFn makes the
			// package available, IncRef will success. Otherwise because there is no
			// preExecFn provided, we will build the package.
			return p.Add(ctx, pkg, nil)
		}

		p.added[pkg.DerivationID] = struct{}{}
		for _, d := range pkg.BuildDependencies {
			if err := p.Add(ctx, d, preExecFn); err != nil {
				return err
			}
		}
		p.newPkgs = append(p.newPkgs, pkg)
		return nil
	case err == nil:
		p.added[pkg.DerivationID] = struct{}{}
		p.availables = append(p.availables, pkg)
		return nil
	default:
		return err
	}
}

// Execute executes packages' derivations added to the plan and all their
// dependencies. All packages will be dereferenced after the build. Leave
// it to the user to decide those of which packages will be used at the
// runtime.
// There may be a chance that a package is removed during the short amount of
// time. But since IncRef will update the last accessed timestamp, cleaning up
// with any reasonable time window (e.g. 1 hour) is highly unlikely to remove
// packages just dereferenced and may be IncRef within seconds. And even if it's
// happened, the caller can retry the process.
func (p *ExecutionPlan) Execute(ctx context.Context, tempDir string, execFn Executor) (err error) {
	if p.executed {
		return ErrExecutionPlanExecuted
	}
	p.executed = true

	for _, pkg := range p.newPkgs {
		if err := pkg.Handler.Build(func() error {
			if err := dumpProto(pkg.Action, pkg.Handler.LoggingDirectory(), "action.pb"); err != nil {
				return err
			}
			if err := dumpProto(pkg.Derivation, pkg.Handler.LoggingDirectory(), "derivation.pb"); err != nil {
				return err
			}

			logging.Infof(ctx, "build package %s", pkg.DerivationID)
			d, err := os.MkdirTemp(tempDir, fmt.Sprintf("%s-", pkg.DerivationID))
			if err != nil {
				return err
			}

			var out strings.Builder
			cfg := &ExecutionConfig{
				OutputDir:  pkg.Handler.OutputDirectory(),
				WorkingDir: d,
				Stdout:     &out,
				Stderr:     &out,
			}
			if err := execFn(ctx, cfg, pkg.Derivation); err != nil {
				logging.Errorf(ctx, "\n%s\n", out.String())
				return err
			}
			logging.Debugf(ctx, "\n%s", out.String())
			return nil
		}); err != nil {
			return fmt.Errorf("failed to build package: %s: %w", pkg.DerivationID, err)
		}
		if err := pkg.Handler.IncRef(); err != nil {
			return fmt.Errorf("failed to reference the package: %s: %w", pkg.DerivationID, err)
		}
		p.availables = append(p.availables, pkg)
	}

	return
}

// Release releases all packages referenced by the build plan.
func (p *ExecutionPlan) Release() error {
	for len(p.availables) != 0 {
		pkg := p.availables[0]
		if err := pkg.Handler.DecRef(); err != nil {
			return err
		}
		p.availables = p.availables[1:]
	}
	return nil
}

func dumpProto(m protoreflect.ProtoMessage, dir, name string) error {
	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := protojson.MarshalOptions{Multiline: true}.Marshal(m)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	return nil
}

type Builder struct {
	platforms generators.Platforms
	packages  core.PackageManager
	processor *actions.ActionProcessor

	preExecFn PreExecuteHook
	execFn    Executor
}

// NewBuilder creates a Builder to manage the standard build workflow for
// actions.Package by converting generators.Generator to actions.Package and
// execute the *core.Derivation to make the package available locally.
func NewBuilder(plats generators.Platforms, pm core.PackageManager, ap *actions.ActionProcessor) *Builder {
	return &Builder{
		platforms: plats,
		packages:  pm,
		processor: ap,

		execFn: Execute,
	}
}

// SetPreExecuteHook sets the PreExecuteHook for Builder before execution.
// See also: ExecutionPlan.
func (b *Builder) SetPreExecuteHook(preExecFn PreExecuteHook) {
	b.preExecFn = preExecFn
}

// SetExecutor sets the Executor for Builder when execution *core.Derivation.
// See also: ExecutionPlan.
func (b *Builder) SetExecutor(execFn Executor) {
	b.execFn = execFn
}

// Build triggers the standard workflow converting generators.Generator to
// actions.Package which are available in the storage.
func (b *Builder) Build(ctx context.Context, buildTempDir string, g generators.Generator) (actions.Package, error) {
	pkgs, err := b.BuildAll(ctx, buildTempDir, []generators.Generator{g})
	if err != nil {
		return actions.Package{}, err
	}
	return pkgs[0], nil
}

// BuildAll triggers the standard workflow converting []generators.Generator to
// []actions.Package which are available in the storage.
func (b *Builder) BuildAll(ctx context.Context, buildTempDir string, gs []generators.Generator) ([]actions.Package, error) {
	// generators.Generator -> *core.Action
	var acts []*core.Action
	for _, g := range gs {
		a, err := g.Generate(ctx, b.platforms)
		if err != nil {
			return nil, err
		}

		acts = append(acts, a)
	}

	// *core.Action -> actions.Package
	var pkgs []actions.Package
	for _, a := range acts {
		pkg, err := b.processor.Process(b.platforms.Build.String(), b.packages, a)
		if err != nil {
			return nil, err
		}
		pkgs = append(pkgs, pkg)
	}

	// Make actions.Package available
	plan := NewExecutionPlan(ctx)
	defer plan.Release()
	for _, pkg := range pkgs {
		if err := plan.Add(ctx, pkg, b.preExecFn); err != nil {
			return nil, err
		}
	}
	if err := plan.Execute(ctx, buildTempDir, b.execFn); err != nil {
		return nil, err
	}

	return pkgs, nil
}
