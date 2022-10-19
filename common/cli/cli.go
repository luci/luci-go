// Copyright 2016 The LUCI Authors.
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

// Package cli is a helper package for "github.com/maruel/subcommands".
//
// It adds a non-intrusive integration with context.Context.
package cli

import (
	"context"
	"io"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/system/environ"
)

// ContextModificator takes a context, adds something, and returns a new one.
//
// It is implemented by Application and can optionally by implemented by
// subcommands.CommandRun instances. It will be called by GetContext to modify
// initial context.
type ContextModificator interface {
	ModifyContext(context.Context) context.Context
}

// GetContext sniffs ContextModificator in the app and in the cmd and uses them
// to derive a context for the command.
//
// Subcommands can use it to get an initial context in their `Run` methods.
//
// Uses `env` to initialize luci/common/system/environ environment in the
// context. In particular populates unset (but registered in the CLI app) env
// vars with their default values (if they are not empty).
//
// Order of the context modifications:
//  1. Start with the background context.
//  2. Apply `app` modifications if `app` implements ContextModificator.
//  3. Initialize luci/common/system/environ in the context.
//  4. Apply `cmd` modifications if `cmd` implements ContextModificator.
//
// In particular, command's modificator is able to examine env vars in
// the context.
func GetContext(app subcommands.Application, cmd subcommands.CommandRun, env subcommands.Env) context.Context {
	ctx := context.Background()
	if m, _ := app.(ContextModificator); m != nil {
		ctx = m.ModifyContext(ctx)
	}

	root := environ.FromCtx(ctx)
	for name, val := range env {
		if val.Value != "" || val.Exists {
			root.Set(name, val.Value)
		}
	}
	ctx = root.SetInCtx(ctx)

	if m, _ := cmd.(ContextModificator); m != nil {
		ctx = m.ModifyContext(ctx)
	}

	return ctx
}

// Application is like subcommands.DefaultApplication, except it also implements
// ContextModificator.
type Application struct {
	Name     string
	Title    string
	Context  func(context.Context) context.Context
	Commands []*subcommands.Command
	EnvVars  map[string]subcommands.EnvVarDefinition

	profiling profilingExt // empty struct if 'include_profiler' build tag is not set
}

var _ interface {
	subcommands.Application
	ContextModificator
} = (*Application)(nil)

// GetName implements interface subcommands.Application.
func (a *Application) GetName() string {
	return a.Name
}

// GetTitle implements interface subcommands.Application.
func (a *Application) GetTitle() string {
	return a.Title
}

// GetCommands implements interface subcommands.Application.
func (a *Application) GetCommands() []*subcommands.Command {
	a.profiling.addProfiling(a.Commands)
	return a.Commands
}

// GetOut implements interface subcommands.Application.
func (a *Application) GetOut() io.Writer {
	return os.Stdout
}

// GetErr implements interface subcommands.Application.
func (a *Application) GetErr() io.Writer {
	return os.Stderr
}

// GetEnvVars implements interface subcommands.Application.
func (a *Application) GetEnvVars() map[string]subcommands.EnvVarDefinition {
	return a.EnvVars
}

// ModifyContext implements interface ContextModificator.
//
// 'ctx' here is always context.Background().
func (a *Application) ModifyContext(ctx context.Context) context.Context {
	if a.Context != nil {
		return a.Context(ctx)
	}
	return ctx
}
