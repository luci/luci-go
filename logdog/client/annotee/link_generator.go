// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package annotee

import (
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/logdog/common/viewer"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"
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
	Project cfgtypes.ProjectName
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
