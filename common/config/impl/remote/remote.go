// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package remote

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"golang.org/x/net/context"

	config "github.com/luci/luci-go/common/config"
	genApi "github.com/luci/luci-go/common/config/impl/remote/generated_api/v1"
)

// Use adds an implementation of the config service which talks to the actual
// luci-config service.
// You may pass in a custom http client.
func Use(c context.Context, client *http.Client, basePath string) context.Context {
	if client == nil {
		client = http.DefaultClient
	}

	return config.SetFactory(c, func(ic context.Context) config.Interface {
		service, err := genApi.New(client)
		if basePath != "" {
			service.BasePath = basePath
		}
		if err != nil {
			// client is nil
			panic("unreachable")
		}
		return &remoteImpl{service}
	})
}

type remoteImpl struct {
	service *genApi.Service
}

func (r *remoteImpl) GetConfig(configSet, path string) (*config.Config, error) {
	resp, err := r.service.GetConfig(configSet, path).Do()
	if err != nil {
		return nil, err
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Content)
	if err != nil {
		return nil, err
	}

	return &config.Config{
		configSet,
		string(decoded),
		resp.ContentHash,
		resp.Revision}, nil
}

func (r *remoteImpl) GetConfigByHash(configSet string) (string, error) {
	resp, err := r.service.GetConfigByHash(configSet).Do()
	if err != nil {
		return "", err
	}

	decoded, err := base64.StdEncoding.DecodeString(resp.Content)
	if err != nil {
		return "", err
	}

	return string(decoded), nil
}

func (r *remoteImpl) GetConfigSetLocation(configSet string) (*url.URL, error) {
	if configSet == "" {
		return nil, fmt.Errorf("configSet must be a non-empty string")
	}
	resp, err := r.service.GetMapping().ConfigSet(configSet).Do()
	if err != nil {
		return nil, err
	}

	urlString := "sentinel"
	for _, mapping := range resp.Mappings {
		if mapping.ConfigSet == configSet {
			if urlString != "sentinel" {
				return nil, fmt.Errorf(
					"Duplicate entries '%s' and '%s' for location of config set %s",
					urlString, mapping.Location, configSet)
			}

			urlString = mapping.Location
		}
	}

	url, err := url.Parse(urlString)
	if err != nil {
		return nil, err
	}

	return url, nil
}

func (r *remoteImpl) GetProjects() ([]*config.Project, error) {
	resp, err := r.service.GetProjects().Do()
	if err != nil {
		return nil, err
	}

	projects := make([]*config.Project, len(resp.Projects))
	for i, p := range resp.Projects {
		repoType := parseWireRepoType(p.RepoType)

		url, err := url.Parse(p.RepoUrl)
		if err != nil {
			return nil, err
		}

		projects[i] = &config.Project{
			p.Id,
			p.Name,
			repoType,
			url,
		}
	}
	return projects, err
}

func (r *remoteImpl) GetProjectConfigs(path string) ([]*config.Config, error) {
	resp, err := r.service.GetProjectConfigs(path).Do()
	if err != nil {
		return nil, err
	}

	return convertMultiWireConfigs(resp)
}

func (r *remoteImpl) GetRefConfigs(path string) ([]*config.Config, error) {
	resp, err := r.service.GetRefConfigs(path).Do()
	if err != nil {
		return nil, err
	}

	return convertMultiWireConfigs(resp)
}

func (r *remoteImpl) GetRefs(projectID string) ([]string, error) {
	resp, err := r.service.GetRefs(projectID).Do()
	if err != nil {
		return nil, err
	}

	refs := make([]string, len(resp.Refs))
	for i, ref := range resp.Refs {
		refs[i] = ref.Name
	}
	return refs, err
}

// convertMultiWireConfigs is a utility to convert what we get over the wire
// into the structs we use in the config package.
func convertMultiWireConfigs(wireConfigs *genApi.LuciConfigGetConfigMultiResponseMessage) ([]*config.Config, error) {
	configs := make([]*config.Config, len(wireConfigs.Configs))
	for i, c := range wireConfigs.Configs {
		decoded, err := base64.StdEncoding.DecodeString(c.Content)
		if err != nil {
			return nil, err
		}

		configs[i] = &config.Config{
			c.ConfigSet,
			string(decoded),
			c.ContentHash,
			c.Revision,
		}
	}

	return configs, nil
}

// parseWireRepoType parses the string received over the wire from
// the luci-config service that represents the repo type.
func parseWireRepoType(s string) config.RepoType {
	if s == string(config.GitilesRepo) {
		return config.GitilesRepo
	}

	return config.UnknownRepo
}
