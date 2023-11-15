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

// Package erroring implements a backend that always returns an error for all
// of its calls.
package erroring

import (
	"context"

	"go.chromium.org/luci/config"
)

// New creates a new erroring interface. This interface will always return an
// error for every API call.
//
// The supplied error must not be nil. If it is, New will panic.
func New(err error) config.Interface {
	if err == nil {
		panic("error cannot be nil")
	}
	return erroringInterface{err}
}

type erroringInterface struct {
	err error
}

func (i erroringInterface) GetConfig(ctx context.Context, configSet config.Set, path string, metaOnly bool) (*config.Config, error) {
	return nil, i.err
}

func (i erroringInterface) GetConfigs(ctx context.Context, configSet config.Set, filter func(path string) bool, metaOnly bool) (map[string]config.Config, error) {
	return nil, i.err
}

func (i erroringInterface) GetProjectConfigs(ctx context.Context, path string, metaOnly bool) ([]config.Config, error) {
	return nil, i.err
}

func (i erroringInterface) GetProjects(ctx context.Context) ([]config.Project, error) {
	return nil, i.err
}

func (i erroringInterface) ListFiles(ctx context.Context, configSet config.Set) ([]string, error) {
	return nil, i.err
}

func (i erroringInterface) Close() error {
	return nil
}
