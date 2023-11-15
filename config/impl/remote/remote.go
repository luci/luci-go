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

// Package remote implements backends for config client which will make calls
// to the real Config Service.
package remote

import (
	"compress/zlib"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"google.golang.org/api/googleapi"

	configApi "go.chromium.org/luci/common/api/luci_config/config/v1"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/config"
)

// ClientFactory returns HTTP client to use (given a context).
//
// See 'NewV1' for more details.
type ClientFactory func(context.Context) (*http.Client, error)

// NewV1 returns an implementation of the config service which talks to the
// actual luci-config service v1 using given transport.
//
// configServiceURL is usually "https://<host>/_ah/api/config/v1/".
//
// ClientFactory returns http.Clients to use for requests (given incoming
// contexts). It's required mostly to support GAE environment, where round
// trippers are bound to contexts and carry RPC deadlines.
//
// If 'clients' is nil, http.DefaultClient will be used for all requests.
func NewV1(host string, insecure bool, clients ClientFactory) config.Interface {
	if clients == nil {
		clients = func(context.Context) (*http.Client, error) {
			return http.DefaultClient, nil
		}
	}

	serviceURL := url.URL{
		Scheme: "https",
		Host:   host,
		Path:   "/_ah/api/config/v1/",
	}
	if insecure {
		serviceURL.Scheme = "http"
	}

	return &remoteImpl{
		serviceURL: serviceURL.String(),
		clients:    clients,
	}
}

type remoteImpl struct {
	serviceURL string
	clients    ClientFactory
}

// service returns Cloud Endpoints API client bound to the given context.
//
// It inherits context's deadline and transport.
func (r *remoteImpl) service(ctx context.Context) (*configApi.Service, error) {
	client, err := r.clients(ctx)
	if err != nil {
		return nil, err
	}
	service, err := configApi.New(client)
	if err != nil {
		return nil, err
	}

	service.BasePath = r.serviceURL
	return service, nil
}

func (r *remoteImpl) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	srv, err := r.service(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := srv.GetConfig(string(configSet), path).HashOnly(metaOnly).UseZlib(true).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}

	var decoded []byte
	if !metaOnly {
		if resp.IsZlibCompressed {
			reader, err := zlib.NewReader(base64.NewDecoder(base64.StdEncoding, strings.NewReader(resp.Content)))
			if err != nil {
				return nil, err
			}
			if decoded, err = io.ReadAll(reader); err != nil {
				return nil, err
			}
		} else {
			if decoded, err = base64.StdEncoding.DecodeString(resp.Content); err != nil {
				return nil, err
			}
		}
	}

	return &config.Config{
		Meta: config.Meta{
			ConfigSet:   configSet,
			Path:        path,
			ContentHash: resp.ContentHash,
			Revision:    resp.Revision,
			ViewURL:     resp.Url,
		},
		Content: string(decoded),
	}, nil
}

func (r *remoteImpl) GetConfigs(ctx context.Context, cfgSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	return nil, errors.New("this method is not supported when using v1 API")
}

func (r *remoteImpl) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	srv, err := r.service(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := srv.GetConfigSets().ConfigSet(string(configSet)).IncludeFiles(true).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}
	var files []string
	for _, cs := range resp.ConfigSets {
		for _, fs := range cs.Files {
			files = append(files, fs.Path)
		}
	}
	sort.Strings(files)
	return files, nil
}

func (r *remoteImpl) GetProjects(ctx context.Context) ([]config.Project, error) {
	srv, err := r.service(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := srv.GetProjects().Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}

	projects := make([]config.Project, len(resp.Projects))
	for i, p := range resp.Projects {
		repoType := parseWireRepoType(p.RepoType)

		url, err := url.Parse(p.RepoUrl)
		if err != nil {
			lc := logging.SetField(ctx, "projectID", p.Id)
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

func (r *remoteImpl) Close() error {
	return nil
}

func (r *remoteImpl) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	srv, err := r.service(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := srv.GetProjectConfigs(path).HashesOnly(metaOnly).Context(ctx).Do()
	if err != nil {
		return nil, apiErr(err)
	}

	c := logging.SetField(ctx, "path", path)
	return convertMultiWireConfigs(c, path, resp, metaOnly)
}

// convertMultiWireConfigs is a utility to convert what we get over the wire
// into the structs we use in the config package.
func convertMultiWireConfigs(ctx context.Context, path string, wireConfigs *configApi.LuciConfigGetConfigMultiResponseMessage, metaOnly bool) ([]config.Config, error) {
	configs := make([]config.Config, len(wireConfigs.Configs))
	for i, c := range wireConfigs.Configs {
		var decoded []byte
		var err error

		if !metaOnly {
			decoded, err = base64.StdEncoding.DecodeString(c.Content)
			if err != nil {
				lc := logging.SetField(ctx, "configSet", c.ConfigSet)
				logging.Warningf(lc, "Failed to base64 decode config: %s", err)
			}
		}

		configs[i] = config.Config{
			Meta: config.Meta{
				ConfigSet:   config.Set(c.ConfigSet),
				Path:        path,
				ContentHash: c.ContentHash,
				Revision:    c.Revision,
				ViewURL:     c.Url,
			},
			Content: string(decoded),
			Error:   err,
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
		return transient.Tag.Apply(err)
	}
	return err
}
