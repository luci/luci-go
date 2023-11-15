// Copyright 2023 The LUCI Authors.
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

package gaeconfig

import (
	"context"
	"sync"

	"go.chromium.org/luci/config"
)

// lazyConfigClient is a lazily-constructed configClient that implements
// config.Interface. The client will be initialized by the provided
// clientFactory function.
type lazyConfigClient struct {
	clientFactory func(ctx context.Context) config.Interface

	inner     config.Interface
	innerOnce sync.Once
}

var _ config.Interface = (*lazyConfigClient)(nil)

func newLazyConfigClient(factory func(ctx context.Context) config.Interface) *lazyConfigClient {
	return &lazyConfigClient{
		clientFactory: factory,
	}
}

func (client *lazyConfigClient) lazyInit(ctx context.Context) {
	client.innerOnce.Do(func() {
		client.inner = client.clientFactory(ctx)
	})
}

// GetConfig implements config.Interface.
func (client *lazyConfigClient) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	client.lazyInit(ctx)
	return client.inner.GetConfig(ctx, configSet, path, metaOnly)
}

// GetConfigs implements config.Interface.
func (client *lazyConfigClient) GetConfigs(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	client.lazyInit(ctx)
	return client.inner.GetConfigs(ctx, configSet, filter, metaOnly)
}

// GetProjectConfigs implements config.Interface.
func (client *lazyConfigClient) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	client.lazyInit(ctx)
	return client.inner.GetProjectConfigs(ctx, path, metaOnly)
}

// GetProjects implements config.Interface.
func (client *lazyConfigClient) GetProjects(ctx context.Context) ([]config.Project, error) {
	client.lazyInit(ctx)
	return client.inner.GetProjects(ctx)
}

// ListFiles implements config.Interface.
func (client *lazyConfigClient) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	client.lazyInit(ctx)
	return client.inner.ListFiles(ctx, configSet)
}

// Close implements config.Interface.
func (client *lazyConfigClient) Close() error {
	// This is potentially racy if client is initialized and closed at the same
	// time. But in this case, Close is never called so it's not a big deal.
	if client.inner != nil {
		return client.inner.Close()
	}
	return nil
}
