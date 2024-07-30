// Copyright 2016 The LUCI Authors.
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

package ui

import (
	"sort"
	"strings"
)

// Collection of structs to describe how to lay out the overview pages of Milo.
// There is an implicit hierarchy of Builder Group -> Builder.

// CIService is a backing service for a Continuous Integration system.
type CIService struct {
	// Host points to the specific instance of this CI Service.
	Host *Link

	// BuilderGroups lists all of the known named groups of builders within this service.
	BuilderGroups []BuilderGroup
}

// Sort sorts builder groups by their Name.
func (c *CIService) Sort() {
	sort.Slice(c.BuilderGroups, func(i, j int) bool {
		return c.BuilderGroups[i].Name < c.BuilderGroups[j].Name
	})
}

// BuilderGroup is a container to describe a named cluster of builders.
// In buildbucket CI system, it refers to a Bucket
type BuilderGroup struct {
	// Name is the name of the group.
	Name string
	// Builders is a list of links to the builder page for that builder.
	Builders []Link
}

// Sort sorts Builders by the lowercase of their name.
func (g *BuilderGroup) Sort() {
	sort.Slice(g.Builders, func(i, j int) bool {
		return strings.ToLower(g.Builders[i].Label) < strings.ToLower(g.Builders[j].Label)
	})
}
