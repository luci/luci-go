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
package workflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"sync"

	"go.chromium.org/luci/cipkg/base/actions"
	"go.chromium.org/luci/cipkg/core"
	"go.chromium.org/luci/luciexe/build"
)

type SubstepFn func(ctx context.Context, root *build.Step) error

type substep struct {
	fn   SubstepFn
	done chan error
}

// RootStep manages lifetime of root step.
type RootStep struct {
	id      string
	substep chan substep

	errs  []error
	ended chan struct{}

	initFn func()
}

// NewRootStep creates a root step for managing steps life time in luciexe.
// luciexe step will be lazily created when RunSubstep is called or root step
// ended.
func NewRootStep(ctx context.Context, name, id string) *RootStep {
	r := &RootStep{
		id:      id,
		substep: make(chan substep),
	}
	r.initFn = sync.OnceFunc(func() {
		r.ended = make(chan struct{})
		go r.runRoot(ctx, name)
	})

	return r
}

// ID is the unique ID for the root step.
func (r *RootStep) ID() string { return r.id }

func (r *RootStep) runRoot(ctx context.Context, name string) {
	defer close(r.ended)

	s, ctx := build.ScheduleStep(ctx, name)
	defer func() { s.End(r.Err()) }()

	for sub := range r.substep {
		err := sub.fn(ctx, s)
		r.errs = append(r.errs, err)
		sub.done <- err
	}
}

// IsEnded returns whether the step has been ended.
func (r *RootStep) IsEnded() bool {
	// Haven't started.
	if r.ended == nil {
		return false
	}

	select {
	case <-r.ended:
		return true
	default:
		return false
	}
}

// RunSubstep execute the SubstepFn in the root step environment with its
// context and *build.Step. Substep still needs to create its own step context
// by calling build.StartStep in SubstepFn.
func (r *RootStep) RunSubstep(ctx context.Context, sub SubstepFn) error {
	if r.IsEnded() {
		return fmt.Errorf("root step ended")
	}
	r.initFn()

	done := make(chan error)
	r.substep <- substep{fn: sub, done: done}

	// Either current context or RootStep is canceled/finished.
	select {
	case <-ctx.Done():
		return fmt.Errorf("sub step cancled")
	case err := <-done:
		return err
	}
}

func (r *RootStep) Err() error {
	return errors.Join(r.errs...)
}

func (r *RootStep) End() {
	if r.IsEnded() {
		return
	}
	r.initFn()

	close(r.substep)
	<-r.ended
}

func (r *RootStep) EndWith(err error) {
	if r.IsEnded() {
		return
	}

	// Avoid duplication
	if !errors.Is(r.Err(), err) {
		r.errs = append(r.errs, err)
	}

	r.End()
}

func RunStepCommand(ctx context.Context, cmd *exec.Cmd) (err error) {
	s, _ := build.StartStep(ctx, fmt.Sprintf("run command: %s", cmd.Args))
	defer func() { s.End(err) }()
	stepOutput := s.Log("stdout")

	if cmd.Stdout == nil {
		cmd.Stdout = stepOutput
	} else {
		cmd.Stdout = io.MultiWriter(cmd.Stdout, stepOutput)
	}

	if cmd.Stderr == nil {
		cmd.Stderr = stepOutput
	} else {
		cmd.Stderr = io.MultiWriter(cmd.Stderr, stepOutput)
	}

	fmt.Fprintf(s.Log("execution details"), "%#v\n", cmd)

	err = cmd.Run()
	return
}

type RootSteps map[string]*RootStep

// NewRootSteps creates RootSteps for managing a lookup table for root steps in
// luciexe. This can be updated by preExecFn and used by execFn to group
// derivation execution by root steps.
func NewRootSteps() RootSteps {
	return make(RootSteps)
}

// UpdateRoot sets all package's non root dependencies' root to the package's
// root recursively.
func (rs RootSteps) UpdateRoot(ctx context.Context, pkg actions.Package) (*RootStep, error) {
	return rs.update(ctx, pkg, nil)
}

func (rs RootSteps) update(ctx context.Context, pkg actions.Package, root *RootStep) (*RootStep, error) {
	if r, ok := rs[pkg.ActionID]; ok {
		// If pkg has root other than itself.
		if r.ID() != pkg.ActionID {
			if root == nil {
				return nil, fmt.Errorf("top level package shouldn't belong to other root: %s, from %s", pkg.ActionID, r.ID())
			}
			if r.ID() != root.ID() {
				return nil, fmt.Errorf("package must only belong to one root: %s, from %s and %s", pkg.ActionID, r.ID(), root.ID())
			}
		}

		return r, nil
	}

	if root == nil || isRootStep(pkg) {
		name := pkg.Action.Metadata.GetLuciexe().GetStepName()
		if name == "" {
			name = pkg.ActionID
		}

		root = NewRootStep(ctx, name, pkg.ActionID)
	}
	rs[pkg.ActionID] = root

	for _, dep := range pkg.RuntimeDependencies {
		if _, err := rs.update(ctx, dep, root); err != nil {
			return nil, err
		}
	}

	for _, dep := range pkg.BuildDependencies {
		if _, err := rs.update(ctx, dep, root); err != nil {
			return nil, err
		}
	}

	return root, nil
}

// GetRoot returns the root step for the action id.
func (rs RootSteps) GetRoot(id string) *RootStep { return rs[id] }

func (rs RootSteps) ExecutorConfig() PackageExecutorConfig {
	type rootContextType string
	var rootContext rootContextType = "rootContext"

	return PackageExecutorConfig{
		PreExecFn: func(ctx context.Context, pkg actions.Package) (context.Context, error) {
			r := rs.GetRoot(pkg.ActionID)
			return context.WithValue(ctx, rootContext, r), nil
		},

		ExecFn: func(ctx context.Context, cfg *ExecutionConfig, drv *core.Derivation) error {
			r := ctx.Value(rootContext).(*RootStep)

			err := r.RunSubstep(ctx, func(ctx context.Context, root *build.Step) (err error) {
				s, ctx := build.StartStep(ctx, fmt.Sprintf("build %s", drv.Name))
				defer func() { s.End(err) }()

				stepOutput := s.Log("stdout")
				cmd := exec.CommandContext(ctx, drv.Args[0], drv.Args[1:]...)
				cmd.Path = drv.Args[0]
				cmd.Dir = cfg.WorkingDir
				cmd.Stdin = cfg.Stdin
				cmd.Stdout = io.MultiWriter(stepOutput, cfg.Stdout)
				cmd.Stderr = io.MultiWriter(stepOutput, cfg.Stderr)
				cmd.Env = append(slices.Clone(drv.Env), "out="+cfg.OutputDir)

				fmt.Fprintf(s.Log("execution details"), "%#v\n", cmd)
				err = cmd.Run()

				return
			})

			return err
		},

		PostExecFn: func(ctx context.Context, pkg actions.Package, execErr error) error {
			r := rs.GetRoot(pkg.ActionID)
			if execErr != nil || r.ID() == pkg.ActionID {
				r.EndWith(execErr)
			}
			return nil
		},
	}
}

// isRootStep returns whether a package is a root step in luciexe.
func isRootStep(pkg actions.Package) bool {
	return pkg.Action.Metadata.GetLuciexe().GetStepName() != ""
}
