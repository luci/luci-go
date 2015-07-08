// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"path/filepath"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/common/auth/model"
)

// isGroupMember returns true if a user is in a group.
// This function is provided for tests and ease of recursive call.
// Note that completely ignores error cases not to stop authentication.
// See: https://codereview.chromium.org/1229503002/patch/20001/30001
func isInGroup(groupName, userName string, groups map[string]*model.AuthGroup, seen map[string]bool) bool {
	if groupName == "*" {
		return true
	}
	if seen[groupName] { // loop case
		return false
	}
	seen[groupName] = true
	g, ok := groups[groupName]
	if !ok { // group not found.
		return false
	}

	for _, gb := range g.Globs {
		matched, err := filepath.Match(gb, userName)
		if err != nil {
			// we ignores malformed glob.
			// primary should check malformed glob.
			continue
		}
		if matched {
			return true
		}
	}

	for _, m := range g.Members {
		if m == userName {
			return true
		}
	}

	for _, n := range g.Nested {
		found := isInGroup(n, userName, groups, seen)
		if found {
			return true
		}
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
