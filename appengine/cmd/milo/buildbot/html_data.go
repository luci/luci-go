// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package buildbot

import (
	"github.com/luci/luci-go/appengine/cmd/milo/resp"
	"github.com/luci/luci-go/appengine/cmd/milo/settings"
	"github.com/luci/luci-go/server/templates"
)

// TestableBuild is a subclass of Build that interfaces with TestableHandler and
// includes sample test data.
type TestableBuild struct{ Build }

// TestableBuilder is a subclass of Builder that interfaces with TestableHandler
// and includes sample test data.
type TestableBuilder struct{ Builder }

// TestData returns sample test data.
func (b Build) TestData() []settings.TestBundle {
	return []settings.TestBundle{}
}

// TestData returns sample test data.
func (b Builder) TestData() []settings.TestBundle {
	bb := &resp.MiloBuild{
		SourceStamp: &resp.SourceStamp{
			Commit: resp.Commit{
				Revision: "abcdef",
			},
		},
	}
	return []settings.TestBundle{
		{
			Description: "Basic Test no builds",
			Data: templates.Args{
				"Builder": &resp.MiloBuilder{
					Name: "Sample Builder",
				},
			},
		},
		{
			Description: "Basic Test with builds",
			Data: templates.Args{
				"Builder": &resp.MiloBuilder{
					Name: "Sample Builder",
					CurrentBuilds: []*resp.BuildRef{
						{
							URL:   "Some URL",
							Label: "Some current build",
							Build: bb,
						},
					},
					PendingBuilds: []*resp.BuildRef{
						{
							URL:   "Some URL",
							Label: "Some pending build",
							Build: bb,
						},
					},
					FinishedBuilds: []*resp.BuildRef{
						{
							URL:   "Some URL",
							Label: "Some finished build",
							Build: bb,
						},
					},
				},
			},
		},
	}
}
