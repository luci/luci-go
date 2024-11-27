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

// Package prjcfgtest eases controlling of project configs in test.
//
// In integration tests, prefer setting configs via cfgclient/cfgmemory and
// calling SubmitRefreshTasks and executing all outstanding tasks via tq.
package prjcfgtest

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmemory "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/refresher"
)

// Create creates project config for the first time.
//
// Panics if project config already exists.
func Create(ctx context.Context, project string, cfg *cfgpb.Config) {
	MustNotExist(ctx, project)
	ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
		config.MustProjectSet(project): {prjcfg.ConfigFileName(ctx): prototext.Format(cfg)},
	}))
	err := refresher.UpdateProject(ctx, project, func(context.Context) error { return nil })
	if err != nil {
		panic(err)
	}
}

// Update updates project config to, presumed, newer version.
//
// Panics if project config doesn't exist.
func Update(ctx context.Context, project string, cfg *cfgpb.Config) {
	MustExist(ctx, project)
	ctx = cfgclient.Use(ctx, cfgmemory.New(map[config.Set]cfgmemory.Files{
		config.MustProjectSet(project): {prjcfg.ConfigFileName(ctx): prototext.Format(cfg)},
	}))
	err := refresher.UpdateProject(ctx, project, func(context.Context) error { return nil })
	if err != nil {
		panic(err)
	}
}

// Disable disables project.
//
// Panics if project config doesn't exist.
func Disable(ctx context.Context, project string) {
	MustExist(ctx, project)
	err := refresher.DisableProject(ctx, project, func(context.Context) error { return nil })
	if err != nil {
		panic(err)
	}
}

// Enable enables project.
//
// Panics if project config doesn't exist.
func Enable(ctx context.Context, project string) {
	m := MustExist(ctx, project)
	if _, err := m.GetConfigGroups(ctx); err != nil {
		panic(err)
	}
	p := prjcfg.ProjectConfig{Project: project}
	if err := datastore.Get(ctx, &p); err != nil {
		panic(err)
	}
	p.Enabled = true
	p.EVersion++
	if err := datastore.Put(ctx, &p); err != nil {
		panic(err)
	}
}

// Delete deletes the project config.
//
// Panics if project config doesn't exist.
func Delete(ctx context.Context, project string) {
	m := MustExist(ctx, project)
	cfgs, err := m.GetConfigGroups(ctx)
	if err != nil {
		panic(err)
	}
	if err := datastore.Delete(ctx, &prjcfg.ProjectConfig{Project: project}, cfgs); err != nil {
		panic(err)
	}
}

// MustExist ensures that The Meta of a given project exists. Panics, otherwise.
func MustExist(ctx context.Context, project string) prjcfg.Meta {
	switch m, err := prjcfg.GetLatestMeta(ctx, project); {
	case err != nil:
		panic(err)
	case !m.Exists():
		panic(fmt.Errorf("project %q doesn't exist", project))
	default:
		return m
	}
}

// MustNotExist ensures that The Meta of a given project does not exists.
// Panics, otherwise.
func MustNotExist(ctx context.Context, project string) {
	switch m, err := prjcfg.GetLatestMeta(ctx, project); {
	case err != nil:
		panic(err)
	case m.Exists():
		panic(fmt.Errorf("project %q already exists", project))
	}
}
