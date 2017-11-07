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

// Collection of structs to describe how to lay out the front page of Milo.
// There is an implicit hierarchy of Master -> Builder.

type FrontPage struct {
	// CIServices is a backing service for a Continuous Integration system,
	// such as "buildbot", "swarmbucket", or "dm".
	CIServices []CIService
}

// CIService is a backing service for a Continuous Integration system,
// such as "buildbot", "swarmbucket", or "dm".
type CIService struct {
	// Name is the display name of the service, which could be "Buildbot",
	// "SwarmBucket", or "Dungeon Master"
	Name string

	// Host points to the specific instance of this CI Service.
	Host *Link

	// BuilderGroups lists all of the known named groups of builders within this service.
	BuilderGroups []BuilderGroup
}

// BuilderGroup is a container to describe a named cluster of builders.
// This takes on other names in each CI services:
// Buildbot: Master
// Buildbucket: Bucket
// Dungeon Master: Bucket
type BuilderGroup struct {
	// Name is the name of the group.
	Name string
	// Builders is a list of links to the builder page for that builder.
	Builders []Link
}
