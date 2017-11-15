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

package model

import (
	"fmt"
	"strings"
)

// Link denotes a single labeled link.
type Link struct {
	// Title (text) of the link.
	Label string

	// The destination for the link.
	URL string
}

// GetLinkFromBuildID gets a link given a BuildID, which is the global identifier for a Build.
//
// BuildID is used for indexing BuildSummary and BuilderSummary entities, so this lets us get links
// given BuildSummaries and BuildSummaries in the console, console header, and console lists.
//
// Returns bogus URL in case of error (just "/" + BuildID).
// Depends on buildbucket.ParseBuildAddress to get project.
//
// Buildbot: "buildbot/<mastername>/<buildername>/<buildnumber>"
// Buildbucket: "buildbucket/<buildaddr>"
// For buildbucket, <buildaddr> looks like <bucketname>/<buildername>/<buildnumber> if available
// and <buildid> otherwise.
func GetLinkFromBuildID(b string, project string) string {
	errURL := "/" + b
	parts := strings.Split(b, "/")
	if len(parts) < 2 {
		return errURL
	}

	source := parts[0]
	switch source {
	case "buildbot":
		switch len(parts) {
		case 4:
			return "/" + b
		default:
			return errURL
		}

	case "buildbucket":
		switch {
		case len(parts) == 2 && parts[1] != "":
			return fmt.Sprintf("/p/%s/builds/b%s", project, parts[1])
		case len(parts) == 4:
			return fmt.Sprintf("/p/%s/builders/%s/%s/%s", project, parts[1], parts[2], parts[3])
		default:
			return errURL
		}

	default:
		return errURL
	}
}
