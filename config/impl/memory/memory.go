// Copyright 2015 The LUCI Authors.
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

// Package memory implements in-memory backend for the config client.
//
// May be useful during local development or from unit tests. Do not use in
// production. It is terribly slow.
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config"
)

// Files is all files comprising some config set.
//
// Represented as a mapping from a file path to a config file body.
type Files map[string]string

// SetError artificially pins the error code returned by impl to err. If err is
// nil, impl will behave normally.
//
// impl must be a memory config instance created with New, else SetError will
// panic.
func SetError(impl config.Interface, err error) {
	impl.(*memoryImpl).err = err
}

// New makes an implementation of the config service which takes all configs
// from provided mapping {config set name -> map of configs}. For unit testing.
func New(cfg map[config.Set]Files) config.Interface {
	return &memoryImpl{sets: cfg}
}

type memoryImpl struct {
	sets map[config.Set]Files
	err  error
}

func (m *memoryImpl) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	if err := m.err; err != nil {
		return nil, err
	}

	if set, ok := m.sets[configSet]; ok {
		if cfg := set.configMaybe(configSet, path, metaOnly); cfg != nil {
			return cfg, nil
		}
	}
	return nil, config.ErrNoConfig
}

func (m *memoryImpl) GetConfigs(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	if err := m.err; err != nil {
		return nil, err
	}

	if set, ok := m.sets[configSet]; ok {
		out := make(map[string]config.Config, len(set))
		for path := range set {
			if filter == nil || filter(path) {
				cfg := set.configMaybe(configSet, path, metaOnly)
				if cfg == nil {
					panic("impossible")
				}
				out[path] = *cfg
			}
		}
		return out, nil
	}

	return nil, config.ErrNoConfig
}

func (m *memoryImpl) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	if err := m.err; err != nil {
		return nil, err
	}

	var files []string
	for cf := range m.sets[configSet] {
		files = append(files, cf)
	}
	sort.Strings(files)
	return files, nil
}

func (m *memoryImpl) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	if err := m.err; err != nil {
		return nil, err
	}

	projects, err := m.GetProjects(ctx)
	if err != nil {
		return nil, err
	}
	out := []config.Config{}
	for _, proj := range projects {
		ps, err := config.ProjectSet(proj.ID)
		if err != nil {
			return nil, err
		}
		if cfg, err := m.GetConfig(ctx, ps, path, metaOnly); err == nil {
			out = append(out, *cfg)
		}
	}
	return out, nil
}

func (m *memoryImpl) GetProjects(ctx context.Context) ([]config.Project, error) {
	if err := m.err; err != nil {
		return nil, err
	}

	ids := stringset.New(0)
	for configSet := range m.sets {
		if projID := configSet.Project(); projID != "" {
			ids.Add(projID)
		}
	}
	sorted := ids.ToSlice()
	sort.Strings(sorted)
	out := make([]config.Project, ids.Len())
	for i, id := range sorted {
		out[i] = config.Project{
			ID:       id,
			Name:     strings.Title(id),
			RepoType: config.GitilesRepo,
		}
	}
	return out, nil
}

func (m *memoryImpl) Close() error {
	return nil
}

// configMaybe returns config.Config if such config is in the set, else nil.
func (b Files) configMaybe(configSet config.Set, path string, metaOnly bool) *config.Config {
	if body, ok := b[path]; ok {
		cfg := &config.Config{
			Meta: config.Meta{
				ConfigSet:   configSet,
				Path:        path,
				ContentHash: hash(body),
				Revision:    b.rev(),
				ViewURL:     fmt.Sprintf("https://example.com/view/here/%s", path),
			},
		}
		if !metaOnly {
			cfg.Content = body
		}
		return cfg
	}
	return nil
}

// rev returns fake revision of given config set.
func (b Files) rev() string {
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := sha256.New()
	for _, k := range keys {
		buf.Write([]byte(k))
		buf.Write([]byte{0})
		buf.Write([]byte(b[k]))
		buf.Write([]byte{0})
	}
	return hex.EncodeToString(buf.Sum(nil))[:40]
}

// hash returns generated ContentHash of a config file.
func hash(body string) string {
	buf := sha256.New()
	fmt.Fprintf(buf, "blob %d", len(body))
	buf.Write([]byte{0})
	buf.Write([]byte(body))
	return "v2:" + hex.EncodeToString(buf.Sum(nil))
}
