// Copyright 2021 The LUCI Authors.
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

// Package graph contains groups graph definitions and operations.
package graph

import (
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/auth_service/impl/model"
)

// Graph represents a traversable group graph.
type Graph struct {
	// All graph nodes, key is group name.
	groups map[string]*groupNode
	// All known globs sorted alphabetically.
	// TODO(cjacomet): Sort globs alphabetically
	globs []identity.Glob
	// Group names that directly include the given identity.
	membersIndex map[identity.Identity][]string
	// Group names that directly include the given glob.
	globsIndex map[identity.Glob][]string
}

// groupNode contains information related to an individual group.
type groupNode struct {
	group *model.AuthGroup

	includes []*groupNode // groups directly included by this group.
	included []*groupNode // groups that directly include this group.
}

// initializeNodes initializes the groupNode(s) in the graph
// it creates a groupNode for every group in the datastore.
func (g *Graph) initializeNodes(groups []*model.AuthGroup) {
	for _, group := range groups {
		g.groups[group.ID] = &groupNode{group: group}
		// Populate globs.
		for _, glob := range group.Globs {
			identityGlob := identity.Glob(glob)
			if _, ok := g.globsIndex[identityGlob]; !ok {
				g.globs = append(g.globs, identityGlob)
			}
			g.globsIndex[identityGlob] = append(g.globsIndex[identityGlob], group.ID)
		}

		// Populate members.
		for _, member := range group.Members {
			memberIdentity := identity.Identity(member)
			g.membersIndex[memberIdentity] = append(g.membersIndex[memberIdentity], group.ID)
		}
	}

	// Populate includes and included.
	for _, parent := range groups {
		for _, nestedID := range parent.Nested {
			if nested, ok := g.groups[nestedID]; ok {
				g.groups[parent.ID].includes = append(g.groups[parent.ID].includes, nested)
				nested.included = append(nested.included, g.groups[parent.ID])
			}
		}
	}
}

////////////////////////////////////////////////////////////////////////////////////////

// NewGraph creates all groupNode(s) that are available in the graph.
func NewGraph(groups []*model.AuthGroup) (*Graph, error) {
	graph := &Graph{
		groups:       make(map[string]*groupNode, len(groups)),
		globs:        []identity.Glob{},
		membersIndex: map[identity.Identity][]string{},
		globsIndex:   map[identity.Glob][]string{},
	}

	graph.initializeNodes(groups)

	return graph, nil
}
