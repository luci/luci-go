// Copyright 2015 The LUCI Authors.
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

package hierarchy

import (
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/appengine/coordinator/config"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	"golang.org/x/net/context"
)

func getProjects(c context.Context, r *Request) (*List, error) {
	// None of the projects are streams.
	var l List
	if r.StreamOnly {
		return &l, nil
	}

	// Get all user-accessible active projects.
	projects, err := config.ActiveUserProjects(c)
	if err != nil {
		// If there is an error, we will refrain from filtering projects.
		log.WithError(err).Warningf(c, "Failed to get user project list.")
		return nil, grpcutil.Internal
	}

	next := cfgtypes.ProjectName(r.Next)
	skip := r.Skip
	for _, proj := range projects {
		// Implement "Next" cursor. If set, don't do anything until we've seen it.
		if next != "" {
			if proj == next {
				next = ""
			}
			continue
		}

		// Implement skip.
		if skip > 0 {
			skip--
			continue
		}

		l.Comp = append(l.Comp, &ListComponent{
			Name: string(proj),
		})

		// Implement limit.
		if r.Limit > 0 && len(l.Comp) >= r.Limit {
			l.Next = string(proj)
			break
		}
	}

	return &l, nil
}
