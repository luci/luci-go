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

package gobmap

import (
	"context"
	"errors"
)

// NewConfigVersion loads new project config and updates the map.
func NewConfigVersion(ctx context.Context, project string) error {
	// Call ~config.LatestProjectConfig(project) and for each group, update map as
	// needed.
	return errors.New("not implemented")
}

// Temp. Must be defined in config package.
// project/content_hash/index
type ProjectConfigGroupID string

// Lookup returns ProjectConfigGroupID which watches CLs in the given GoB host, repo,
// ref.
func Lookup(ctx context.Context, host, repo, ref string) ([]ProjectConfigGroupID, error) {
	return errors.New("not implemented")
}
