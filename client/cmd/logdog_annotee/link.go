// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/luci/luci-go/common/config"
	"github.com/luci/luci-go/common/logdog/types"
)

type coordinatorLinkGenerator struct {
	base    types.StreamName
	project config.ProjectName
	prefix  types.StreamName
}

func (g *coordinatorLinkGenerator) canGenerateLinks() bool {
	return (g.base != "" && g.prefix != "")
}

func (g *coordinatorLinkGenerator) GetLink(names ...types.StreamName) string {
	links := make([]string, len(names))
	for i, n := range names {
		streamName := string(g.prefix.Join(n))
		proj := g.project
		if proj == "" {
			proj = "_"
		}
		links[i] = fmt.Sprintf("s=%s", url.QueryEscape(fmt.Sprintf("%s/%s", proj, streamName)))
	}
	return fmt.Sprintf("https://%s.appspot.com/v/?%s", g.base, strings.Join(links, "&"))
}
