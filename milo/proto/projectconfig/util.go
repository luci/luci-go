// Copyright 2017 The LUCI Authors.
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

package projectconfigpb

import (
	"strings"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/milo/internal/utils"
)

// ParseCategory takes a Builder's Category and parses it into a list of
// subcategories. The top-level category is listed first.
func (b *Builder) ParseCategory() []string {
	return strings.Split(b.Category, "|")
}

// GetIdWithFallback returns `id` if its populated. Otherwise, tries to parse
// builder ID from `name`. Returns an error if `name` is not a valid legacy
// builder ID.
//
// TODO(crbug.com/1263768): remove this and use `GetId()` directly once `id`
// field is always populated.
func (b *Builder) GetIdWithFallback() (*buildbucketpb.BuilderID, error) {
	if b.GetId() != nil {
		return b.GetId(), nil
	}
	bid, err := utils.ParseLegacyBuilderID(b.Name)
	if err != nil {
		return nil, err
	}
	return bid, nil
}

// HasUnpopulatedBuilderId returns true if there's at least one builder not
// having ID populated.
//
// TODO(crbug.com/1263768): remove this and use `GetId()` directly once `id
// field is always populated.
func (c *Console) HasUnpopulatedBuilderId() bool {
	for _, b := range c.Builders {

		if b.Id == nil {
			return true
		}
	}
	return false
}

// AllLegacyBuilderIDs returns all BuilderIDs in legacy format mentioned by this
// Console.
func (c *Console) AllLegacyBuilderIDs() ([]string, error) {
	builders := make([]string, 0, len(c.Builders))
	for _, b := range c.Builders {
		bid, err := b.GetIdWithFallback()
		if err != nil {
			return nil, err
		}
		builders = append(builders, utils.LegacyBuilderIDString(bid))
	}
	return builders, nil
}
