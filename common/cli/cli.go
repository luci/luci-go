// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package cli is a helper package for "github.com/maruel/subcommands".
//
// It adds a non-intrusive integration with context.Context.
package cli

import (
	"io"
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"
)

// ContextModificator takes a context, adds something, and returns a new one.
//
// It is implemented by Application and can optionally by implemented by
// subcommands.CommandRun instances. It will be called by GetContext to modify
// initial context.
type ContextModificator interface {
	ModifyContext(context.Context) context.Context
}

var envKey = "holds a subcommands.Env"

// Getenv returns the given value from the embedded subcommands.Env, or "" if
// the value was unset and had no default.
func Getenv(ctx context.Context, key string) string {
	return LookupEnv(ctx, key).Value
}

// LookupEnv returns the given value from the embedded subcommands.Env as-is.
func LookupEnv(ctx context.Context, key string) subcommands.EnvVar {
	e, _ := ctx.Value(&envKey).(subcommands.Env)
	return e[key]
}

// GetContext sniffs ContextModificator in the app and in the cmd and uses them
// to derive a context for the command.
//
// Embeds the subcommands.Env into the Context (if any), which can be accessed
// with the *env methods in this package.
//
// Subcommands can use it to get an initial context in their 'Run' methods.
//
// Returns the background context if app doesn't implement ContextModificator.
func GetContext(app subcommands.Application, cmd subcommands.CommandRun, env subcommands.Env) context.Context {
	ctx := context.Background()
	if m, _ := app.(ContextModificator); m != nil {
		ctx = m.ModifyContext(ctx)
	}
	if m, _ := cmd.(ContextModificator); m != nil {
		ctx = m.ModifyContext(ctx)
	}
	if len(env) > 0 {
		ctx = context.WithValue(ctx, &envKey, env)
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
