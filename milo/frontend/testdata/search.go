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

package testdata

import (
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/server/templates"
)

func Search() []common.TestBundle {
	data := &templates.Args{
		"search": ui.Search{
			CIServices: []ui.CIService{
				{
					Name: "Module 1",
					BuilderGroups: []ui.BuilderGroup{
						{
							Name: "Example master A",
							Builders: []ui.Link{
								*ui.NewLink("Example builder", "/master1/buildera", "Example label"),
								*ui.NewLink("Example builder 2", "/master1/builderb", "Example label 2"),
							},
						},
					},
				},
			},
		},
		"error": "couldn't find ice cream",
	}
	return []common.TestBundle{
		{
			Description: "Basic search page",
			Data:        *data,
		},
	}
}
