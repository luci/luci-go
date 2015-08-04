// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"path/filepath"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/common/auth/model"
)

type flatGroup struct {
	// TODO(yyanagisawa): consider use of regexp for luci-py compatibility.
	//                    [^seq] in go is [!seq] in python.
	globs   []string
	members map[string]bool
}

func flattenGroup(groupName string, groups map[string]*model.AuthGroup, flatGroups map[string]flatGroup, seen map[string]bool) {
	if seen[groupName] {
		return
	}
	seen[groupName] = true
	g, ok := groups[groupName]
	if !ok {
		return
	}

	globs := make(map[string]bool)
	for _, gb := range g.Globs {
		globs[gb] = true
	}

	members := make(map[string]bool)
	for _, m := range g.Members {
		members[m] = true
	}

	for _, n := range g.Nested {
		if n == "*" {
			// Wildcard group that matches all identities (including anonymous!).
			// as
			// https://github.com/luci/luci-py/blob/master/appengine/components/components/auth/api.py#L161
			globs[n] = true
			continue
		}
		flattenGroup(n, groups, flatGroups, seen)
		if nested, ok := flatGroups[n]; ok {
			for _, gb := range nested.globs {
				globs[gb] = true
			}
			for m := range nested.members {
				members[m] = true
			}
		}
	}
	var gs []string
	for gb := range globs {
		gs = append(gs, gb)
	}
	flatGroups[groupName] = flatGroup{
		globs:   gs,
		members: members,
	}
}

func flattenGroups(groups map[string]*model.AuthGroup) map[string]flatGroup {
	flatGroups := make(map[string]flatGroup)
	for n := range groups {
		seen := make(map[string]bool)
		flattenGroup(n, groups, flatGroups, seen)
	}
	return flatGroups
}

func (fg flatGroup) has(userName string) bool {
	if fg.members[userName] {
		return true
	}
	for _, gb := range fg.globs {
		matched, err := filepath.Match(gb, userName)
		if err != nil {
			continue
		}
		if matched {
			return true
		}
	}
	return false
}

// isGroupMember returns true if a user is in a group.
// Currently, this function is called by IsGroupMember, and always flatten
// all groups.  In future change list, this function will only used for
// test purpose.
// Since I believe cost to flatten all groups for test data is negligible,
// this function flatten all groups before checking.
// Note that completely ignores error cases not to stop authentication.
// See: https://codereview.chromium.org/1229503002/patch/20001/30001
func isInGroup(groupName, userName string, groups map[string]*model.AuthGroup, seen map[string]bool) bool {
	if fg, ok := flattenGroups(groups)[groupName]; ok {
		return fg.has(userName)
	}
	return false
}

// IsGroupMember returns true if a member is in a group.
func IsGroupMember(c context.Context, groupName, userName string) (bool, error) {
	// TODO(yyanagisawa): cache coverted data, and match result.
	//                    we do not need to create map everytime.
	snap := &model.AuthDBSnapshot{}
	if err := model.CurrentAuthDBSnapshot(c, snap); err != nil {
		return false, err
	}

	ag := make(map[string]*model.AuthGroup, len(snap.Groups))
	for _, g := range snap.Groups {
		ag[g.Key.StringID()] = g
	}

	seen := make(map[string]bool)
	return isInGroup(groupName, model.ToUserID(userName), ag, seen), nil
}
