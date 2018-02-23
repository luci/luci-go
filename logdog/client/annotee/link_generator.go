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

package annotee

import (
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/logdog/common/viewer"
)

// LinkGenerator generates links for a given log stream.
type LinkGenerator interface {
	// GetLink returns a link for the specified aggregate streams.
	//
	// If no link could be generated, GetLink may return an empty string.
	GetLink(name ...types.StreamName) string
}

// CoordinatorLinkGenerator is a LinkGenerator implementation
type CoordinatorLinkGenerator struct {
	Host    string
	Project types.ProjectName
	Prefix  types.StreamName
}

// CanGenerateLinks returns true if g is sufficiently configured to generate
// LogDog Coordinator links.
func (g *CoordinatorLinkGenerator) CanGenerateLinks() bool {
	return (g.Host != "" && g.Prefix != "")
}

// GetLink implements LinkGenerator.
func (g *CoordinatorLinkGenerator) GetLink(names ...types.StreamName) string {
	paths := make([]types.StreamPath, len(names))
	for i, name := range names {
		paths[i] = g.Prefix.AsPathPrefix(name)
	}
	return viewer.GetURL(g.Host, g.Project, paths...)
}
