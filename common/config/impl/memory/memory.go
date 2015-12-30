// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package memory implements in-memory backend for the config client.
//
// May be useful during local development or from unit tests. Do not use in
// production. It is terribly slow.
package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/stringset"
)

// ConfigSet is a mapping from a file path to a config file body.
type ConfigSet map[string]string

// New makes an implementation of the config service which takes all configs
// from provided mapping {config set name -> map of configs}. For unit testing.
func New(cfg map[string]ConfigSet) config.Interface {
	return &memoryImpl{cfg}
}

// Use adds an implementation of the config service which takes all configs
// from provided mapping {config set name -> map of configs}. For unit testing.
func Use(c context.Context, cfg map[string]ConfigSet) context.Context {
	return config.Set(c, New(cfg))
}

type memoryImpl struct {
	sets map[string]ConfigSet
}

func (m *memoryImpl) GetConfig(configSet, path string, hashOnly bool) (*config.Config, error) {
	if set, ok := m.sets[configSet]; ok {
		if cfg := set.configMaybe(configSet, path, hashOnly); cfg != nil {
			return cfg, nil
		}
	}
	return nil, config.ErrNoConfig
}

func (m *memoryImpl) GetConfigByHash(contentHash string) (string, error) {
	for _, set := range m.sets {
		for _, body := range set {
			if hash(body) == contentHash {
				return body, nil
			}
		}
	}
	return "", config.ErrNoConfig
}

func (m *memoryImpl) GetConfigSetLocation(configSet string) (*url.URL, error) {
	return url.Parse("https://example.com/fake-config/" + configSet)
}

func (m *memoryImpl) GetProjectConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	projects, err := m.GetProjects()
	if err != nil {
		return nil, err
	}
	out := []config.Config{}
	for _, proj := range projects {
		if cfg, err := m.GetConfig("projects/"+proj.ID, path, hashesOnly); err == nil {
			out = append(out, *cfg)
		}
	}
	return out, nil
}

func (m *memoryImpl) GetProjects() ([]config.Project, error) {
	ids := stringset.New(0)
	for configSet := range m.sets {
		chunks := strings.Split(configSet, "/")
		if len(chunks) >= 2 && chunks[0] == "projects" {
			ids.Add(chunks[1])
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

func (m *memoryImpl) GetRefConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	sets := []string{}
	for configSet := range m.sets {
		chunks := strings.Split(configSet, "/")
		if len(chunks) > 3 && chunks[0] == "projects" && chunks[2] == "refs" {
			sets = append(sets, configSet)
		}
	}
	sort.Strings(sets)
	out := []config.Config{}
	for _, configSet := range sets {
		if cfg, err := m.GetConfig(configSet, path, hashesOnly); err == nil {
			out = append(out, *cfg)
		}
	}
	return out, nil
}

func (m *memoryImpl) GetRefs(projectID string) ([]string, error) {
	prefix := "projects/" + projectID + "/"
	out := []string{}
	for configSet := range m.sets {
		if strings.HasPrefix(configSet, prefix) {
			ref := configSet[len(prefix):]
			if strings.HasPrefix(ref, "refs/") {
				out = append(out, ref)
			}
		}
	}
	sort.Strings(out)
	return out, nil
}

// configMaybe returns config.Config is such config is in the set, else nil.
func (b ConfigSet) configMaybe(configSet, path string, hashesOnly bool) *config.Config {
	if body, ok := b[path]; ok {
		cfg := &config.Config{
			ConfigSet:   configSet,
			Path:        path,
			ContentHash: hash(body),
			Revision:    b.rev(),
		}
		if !hashesOnly {
			cfg.Content = body
		}
		return cfg
	}
	return nil
}

// rev returns fake revision of given config set.
func (b ConfigSet) rev() string {
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	buf := sha1.New()
	for _, k := range keys {
		buf.Write([]byte(k))
		buf.Write([]byte{0})
		buf.Write([]byte(b[k]))
		buf.Write([]byte{0})
	}
	return hex.EncodeToString(buf.Sum(nil))
}

// hash returns generated ContentHash of a config file.
func hash(body string) string {
	buf := sha1.New()
	fmt.Fprintf(buf, "blob %d", len(body))
	buf.Write([]byte{0})
	buf.Write([]byte(body))
	return "v1:" + hex.EncodeToString(buf.Sum(nil))
}
