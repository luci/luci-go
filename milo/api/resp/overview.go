// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package resp

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
