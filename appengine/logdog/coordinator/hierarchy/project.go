// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hierarchy

import (
	"github.com/luci/luci-go/appengine/logdog/coordinator"
	"github.com/luci/luci-go/appengine/logdog/coordinator/config"
	luciConfig "github.com/luci/luci-go/common/config"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

func getProjects(c context.Context, r *Request) (*List, error) {
	// None of the projects are streams.
	var l List
	if r.StreamOnly {
		return &l, nil
	}

	projects, err := config.UserProjects(c)
	if err != nil {
		log.WithError(err).Errorf(c, "Failed to get user projects.")
		return nil, err
	}

	// Get all current datastore namespaces.
	nsProjects, err := coordinator.AllProjectsWithNamespaces(c)
	if err != nil {
		// If there is an error, we will refrain from filtering projects.
		log.WithError(err).Warningf(c, "Failed to get namespace project list.")
	} else {
		// Only list projects that have datastore namespaces.
		lookup := make(map[luciConfig.ProjectName]struct{}, len(nsProjects))
		for _, proj := range nsProjects {
			lookup[proj] = struct{}{}
		}

		pos := 0
		for _, proj := range projects {
			if _, ok := lookup[proj]; ok {
				projects[pos] = proj
				pos++
			}
		}
		projects = projects[:pos]
	}

	next := luciConfig.ProjectName(r.Next)
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
