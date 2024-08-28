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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/exec"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/base/generators"
	"go.chromium.org/luci/cipkg/core"
)

type PreExpandHook func(ctx context.Context, pkg actions.Package) error
type PreExecuteHook func(ctx context.Context, pkg actions.Package) (context.Context, error)
type PostExecuteHook func(ctx context.Context, pkg actions.Package, execErr error) error

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

// PackageExecutor is the executor for package to be built by running executor.
// It will ensure all package's dependencies become available in storage when
// the package is executed.
type PackageExecutor struct {
	// Refs are all available packages being referenced by executor. After a
	// package being built, it will also be added to refs.
	// Mapping derivation id to package.
	refs map[string]actions.Package

	preExpandFn PreExpandHook
	preExecFn   PreExecuteHook
	postExecFn  PostExecuteHook

	execFn Executor

	// tempDir is the temporary directory used as the working directory during
	// execution. Since artifacts should be installed into output directory at the
	// end of the execution, we can use the path of temporary directory to detect
	// if any path burned into outputs may affect portability or being potential
	// subjects for runtime rewrite.
	tempDir string
}

// NewPackageExecutor creates a package executor to make packages available.
// All hooks and execFn are optional.
// - preExpandFn can be provided to e.g. fetch package from remote cache.
// - preExecFn can be provided to e.g. setup execution environment.
// - postExecFn can be provided to e.g. cleanup execution environment.
// - If execFn is nil, builder.Execute will be used.
// tempDir will be used as the working directory during execution and can be
// removed by caller after execution.
func NewPackageExecutor(tempDir string, preExpandFn PreExpandHook, preExecFn PreExecuteHook, postExecFn PostExecuteHook, execFn Executor) *PackageExecutor {
	if execFn == nil {
		execFn = Execute
	}
	return &PackageExecutor{
		refs: make(map[string]actions.Package),

		preExpandFn: preExpandFn,
		preExecFn:   preExecFn,
		postExecFn:  postExecFn,

		execFn: execFn,

		tempDir: tempDir,
	}
}

// Expand expands a package with all its dependencies to a flattened slice of
// packages for execution. If preExpandFn is not empty, it will be executed to
// possibly make the package available so its dependencies don't need to be
// expanded.
// preExpandFn will be invoked on the package before on it's dependencies.
func (p *PackageExecutor) Expand(ctx context.Context, pkg actions.Package) ([]actions.Package, error) {
	if _, ok := p.refs[pkg.DerivationID]; ok {
		return []actions.Package{pkg}, nil
	}

	// PreExpandHook is promised to be executed on the package before all its
	// dependencies.
	if p.preExpandFn != nil {
		if err := p.preExpandFn(ctx, pkg); err != nil {
			return nil, fmt.Errorf("failed to run preExpand hook for the package: %s: %w", pkg.ActionID, err)
		}
	}

	var newPkgs []actions.Package

	// Expand runtime dependencies.
	for _, d := range pkg.RuntimeDependencies {
		pkgs, err := p.Expand(ctx, d)
		if err != nil {
			return nil, err
		}
		newPkgs = append(newPkgs, pkgs...)
	}

	switch err := pkg.Handler.IncRef(); {
	case errors.Is(err, core.ErrPackageNotExist):
		for _, d := range pkg.BuildDependencies {
			pkgs, err := p.Expand(ctx, d)
			if err != nil {
				return nil, err
			}
			newPkgs = append(newPkgs, pkgs...)
		}
		newPkgs = append(newPkgs, pkg)
		return newPkgs, nil
	case err == nil:
		p.refs[pkg.DerivationID] = pkg
		newPkgs = append(newPkgs, pkg)
		return newPkgs, nil
	default:
		return nil, err
	}
}

// Execute executes packages' derivations and keeps the reference to the
// package. PreExecHook and PostExecHook, if not nil, are invoked before and
// after the execution. If PreExecHook returns error, Execute will be skipped
// but PostExecHook should be invoked with the error.
func (p *PackageExecutor) Execute(ctx context.Context, pkg actions.Package) error {
	var errs error
	if p.preExecFn != nil {
		ctx, errs = p.preExecFn(ctx, pkg)
	}
	if errs == nil {
		errs = errors.Join(errs, p.execute(ctx, pkg))
	}
	if p.postExecFn != nil {
		errs = errors.Join(errs, p.postExecFn(ctx, pkg, errs))
	}
	return errs
}

func (p *PackageExecutor) execute(ctx context.Context, pkg actions.Package) error {
	// Skip execution if we have referenced the derivation outputs.
	if _, ok := p.refs[pkg.DerivationID]; ok {
		return nil
	}

	if err := pkg.Handler.Build(func() error {
		if err := dumpProto(pkg.Action, pkg.Handler.LoggingDirectory(), "action.pb"); err != nil {
			return err
		}
		if err := dumpProto(pkg.Derivation, pkg.Handler.LoggingDirectory(), "derivation.pb"); err != nil {
			return err
		}

		// Ensure all build dependencies are referenced by the PackageExecutor.
		for _, dep := range pkg.BuildDependencies {
			if _, ok := p.refs[dep.DerivationID]; !ok {
				return fmt.Errorf("dependency not available: %s", dep.DerivationID)
			}
		}

		logging.Infof(ctx, "build package %s", pkg.DerivationID)
		d, err := os.MkdirTemp(p.tempDir, fmt.Sprintf("%s-", pkg.DerivationID))
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
		if err := p.execFn(ctx, cfg, pkg.Derivation); err != nil {
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
	p.refs[pkg.DerivationID] = pkg

	return nil
}

// Release releases all packages referenced by the PackageExecutor.
func (p *PackageExecutor) Release() error {
	var errs error
	for id, pkg := range p.refs {
		if err := pkg.Handler.DecRef(); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		delete(p.refs, id)
	}
	return errs
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
}

// NewBuilder creates a Builder to manage the standard build workflow for
// actions.Package by converting generators.Generator to actions.Package and
// execute the *core.Derivation to make the package available locally.
func NewBuilder(plats generators.Platforms, pm core.PackageManager, ap *actions.ActionProcessor) *Builder {
	return &Builder{
		platforms: plats,
		packages:  pm,
		processor: ap,
	}
}

// Build triggers the standard workflow converting generators.Generator to
// actions.Package which are available in the storage.
func (b *Builder) Build(ctx context.Context, pe *PackageExecutor, g generators.Generator) (actions.Package, error) {
	pkgs, err := b.BuildAll(ctx, pe, []generators.Generator{g})
	if err != nil {
		return actions.Package{}, err
	}
	return pkgs[0], nil
}

// BuildAll triggers the standard workflow converting []generators.Generator to
// []actions.Package which are available in the storage.
func (b *Builder) BuildAll(ctx context.Context, pe *PackageExecutor, gs []generators.Generator) ([]actions.Package, error) {
	pkgs, err := b.GeneratePackages(ctx, gs)
	if err != nil {
		return nil, err
	}

	if err := b.BuildPackages(ctx, pe, pkgs, false); err != nil {
		return nil, err
	}

	return pkgs, nil
}

// GeneratePackages triggers the standard workflow converting
// []generators.Generator to []actions.Package without building them.
func (b *Builder) GeneratePackages(ctx context.Context, gs []generators.Generator) ([]actions.Package, error) {
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

	return pkgs, nil
}

// BuildPackages builds the packages and make all packages available in the
// storage. All packages will be dereferenced after the build. Leave
// it to the user to decide those of which packages will be used at the
// runtime.
// There may be a chance that a package is removed during the short amount of
// time. But since IncRef will update the last accessed timestamp, cleaning up
// with any reasonable time window (e.g. 1 hour) is highly unlikely to remove
// packages just dereferenced and may be IncRef within seconds. And even if it's
// happened, the caller can retry the process.
func (b *Builder) BuildPackages(ctx context.Context, pe *PackageExecutor, pkgs []actions.Package, continueOnExecError bool) error {
	var newPkgs []actions.Package

	defer func() { _ = pe.Release() }()

	var errs error

	added := stringset.New(len(pkgs))
	for _, pkg := range pkgs {
		expands, err := pe.Expand(ctx, pkg)
		if err != nil {
			return err
		}

		for _, expand := range expands {
			if added.Add(expand.ActionID) {
				newPkgs = append(newPkgs, expand)
			}
		}
	}

	executed := stringset.New(len(newPkgs))

	for _, pkg := range newPkgs {
		if !executed.Add(pkg.DerivationID) {
			continue
		}

		if err := pe.Execute(ctx, pkg); err != nil {
			if continueOnExecError {
				errs = errors.Join(errs, err)
				continue
			} else {
				return err
			}
		}
	}

	return errs
}
