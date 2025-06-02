// Copyright 2020 The LUCI Authors.
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

// Package resolving implements an interface that resolves ${var} placeholders
// in config set names and file paths before forwarding calls to some other
// interface.
package resolving

import (
	"context"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/vars"
)

// New creates a config.Interface that resolves ${var} placeholders in config
// set names and file paths before forwarding calls to `next`.
func New(vars *vars.VarSet, next config.Interface) config.Interface {
	return &resolvingInterface{vars, next}
}

type resolvingInterface struct {
	vars *vars.VarSet
	next config.Interface
}

func (r *resolvingInterface) configSet(ctx context.Context, cs config.Set) (config.Set, error) {
	out, err := r.vars.RenderTemplate(ctx, string(cs))
	if err != nil {
		return "", errors.Fmt("bad configSet %q: %w", cs, err)
	}
	return config.Set(out), nil
}

func (r *resolvingInterface) path(ctx context.Context, p string) (string, error) {
	out, err := r.vars.RenderTemplate(ctx, p)
	if err != nil {
		return "", errors.Fmt("bad path %q: %w", p, err)
	}
	return out, nil
}

func (r *resolvingInterface) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	configSet, err := r.configSet(ctx, configSet)
	if err != nil {
		return nil, err
	}
	path, err = r.path(ctx, path)
	if err != nil {
		return nil, err
	}
	return r.next.GetConfig(ctx, configSet, path, metaOnly)
}

func (r *resolvingInterface) GetConfigs(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	configSet, err := r.configSet(ctx, configSet)
	if err != nil {
		return nil, err
	}
	return r.next.GetConfigs(ctx, configSet, filter, metaOnly)
}

func (r *resolvingInterface) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	path, err := r.path(ctx, path)
	if err != nil {
		return nil, err
	}
	return r.next.GetProjectConfigs(ctx, path, metaOnly)
}

func (r *resolvingInterface) GetProjects(ctx context.Context) ([]config.Project, error) {
	return r.next.GetProjects(ctx)
}

func (r *resolvingInterface) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	configSet, err := r.configSet(ctx, configSet)
	if err != nil {
		return nil, err
	}
	return r.next.ListFiles(ctx, configSet)
}

func (r *resolvingInterface) Close() error {
	if r.next != nil {
		return r.next.Close()
	}
	return nil
}
