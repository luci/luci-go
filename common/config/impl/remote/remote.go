// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package remote

import (
	"encoding/base64"
	"fmt"
	"net/url"

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"

	configApi "github.com/luci/luci-go/common/api/luci_config/config/v1"
	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/transport"
)

// New returns an implementation of the config service which talks to the actual
// luci-config service. Uses transport injected into the context for
// authentication. See common/transport.
func New(c context.Context, basePath string) config.Interface {
	service, err := configApi.New(transport.GetClient(c))
	if err != nil {
		// client is nil
		panic("unreachable")
	}
	if basePath != "" {
		service.BasePath = basePath
	}
	return &remoteImpl{service, c}
}

// Use adds an implementation of the config service which talks to the actual
// luci-config service. Uses transport injected into the context for
// authentication. See common/transport.
func Use(c context.Context, basePath string) context.Context {
	return config.SetFactory(c, func(ic context.Context) config.Interface {
		return New(ic, basePath)
	})
}

type remoteImpl struct {
	service *configApi.Service
	c       context.Context
}

func (r *remoteImpl) GetConfig(configSet, path string, hashOnly bool) (*config.Config, error) {
	resp, err := r.service.GetConfig(configSet, path).HashOnly(hashOnly).Do()
	if err != nil {
		return nil, apiErr(err)
	}

	var decoded []byte
	if !hashOnly {
		decoded, err = base64.StdEncoding.DecodeString(resp.Content)
		if err != nil {
			return nil, err
		}
	}

	return &config.Config{
		configSet,
		nil,
		string(decoded),
		resp.ContentHash,
		resp.Revision}, nil
}

func (r *remoteImpl) GetConfigByHash(configSet string) (string, error) {
	resp, err := r.service.GetConfigByHash(configSet).Do()
	if err != nil {
		return "", apiErr(err)
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
		return nil, apiErr(err)
	}

	urlString := "sentinel"
	for _, mapping := range resp.Mappings {
		if mapping.ConfigSet == configSet {
			if urlString != "sentinel" {
				return nil, fmt.Errorf(
					"duplicate entries %q and %q for location of config set %s",
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

func (r *remoteImpl) GetProjects() ([]config.Project, error) {
	resp, err := r.service.GetProjects().Do()
	if err != nil {
		return nil, apiErr(err)
	}

	projects := make([]config.Project, len(resp.Projects))
	for i, p := range resp.Projects {
		repoType := parseWireRepoType(p.RepoType)

		url, err := url.Parse(p.RepoUrl)
		if err != nil {
			lc := logging.SetField(r.c, "projectID", p.Id)
			logging.Warningf(lc, "Failed to parse repo URL %q: %s", p.RepoUrl, err)
		}

		projects[i] = config.Project{
			p.Id,
			p.Name,
			repoType,
			url,
		}
	}
	return projects, err
}

func (r *remoteImpl) GetProjectConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	resp, err := r.service.GetProjectConfigs(path).HashesOnly(hashesOnly).Do()
	if err != nil {
		return nil, apiErr(err)
	}
	c := logging.SetField(r.c, "path", path)
	return convertMultiWireConfigs(c, resp, hashesOnly)
}

func (r *remoteImpl) GetRefConfigs(path string, hashesOnly bool) ([]config.Config, error) {
	resp, err := r.service.GetRefConfigs(path).HashesOnly(hashesOnly).Do()
	if err != nil {
		return nil, apiErr(err)
	}
	c := logging.SetField(r.c, "path", path)
	return convertMultiWireConfigs(c, resp, hashesOnly)
}

func (r *remoteImpl) GetRefs(projectID string) ([]string, error) {
	resp, err := r.service.GetRefs(projectID).Do()
	if err != nil {
		return nil, apiErr(err)
	}

	refs := make([]string, len(resp.Refs))
	for i, ref := range resp.Refs {
		refs[i] = ref.Name
	}
	return refs, err
}

// convertMultiWireConfigs is a utility to convert what we get over the wire
// into the structs we use in the config package.
func convertMultiWireConfigs(ctx context.Context, wireConfigs *configApi.LuciConfigGetConfigMultiResponseMessage, hashesOnly bool) ([]config.Config, error) {
	configs := make([]config.Config, len(wireConfigs.Configs))
	for i, c := range wireConfigs.Configs {
		var decoded []byte
		var err error

		if !hashesOnly {
			decoded, err = base64.StdEncoding.DecodeString(c.Content)
			if err != nil {
				lc := logging.SetField(ctx, "configSet", c.ConfigSet)
				logging.Warningf(lc, "Failed to base64 decode config: %s", err)
			}
		}

		configs[i] = config.Config{
			c.ConfigSet,
			err,
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

// apiErr converts googleapi.Error to an appropriate type.
func apiErr(e error) error {
	err, ok := e.(*googleapi.Error)
	if !ok {
		return e
	}
	if err.Code == 404 {
		return config.ErrNoConfig
	}
	if err.Code >= 500 {
		return errors.WrapTransient(err)
	}
	return err
}
