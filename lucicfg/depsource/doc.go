// Copyright 2025 The LUCI Authors.
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

// Package depsource has interfaces and implementations for lucicfg's
// dependency fetching functionality.
package depsource

import (
	"context"
	"io"
)

// Fetcher describes the core depsource API.
//
// Each package dependency (e.g. '@some-package') will have a single associated
// Fetcher that will be used for loads relative to that package's root.
//
// Notable implementations:
//   - [go.chromium.org/luci/lucicfg/depsource/gitsource] - Manages fetching of
//     remote git repo data.
//   - [go.chromium.org/luci/lucicfg/depsource/gitlocalsource] - Manages fetching
//     of data from a local git checkout.
//   - [go.chromium.org/luci/lucicfg/depsource/filesource] - Manages fetching of
//     local file data.
type Fetcher interface {
	Read(ctx context.Context, pkgRelPath string) (io.ReadCloser, error)
}
