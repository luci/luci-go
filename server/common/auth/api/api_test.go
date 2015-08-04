// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"testing"

	"github.com/luci/luci-go/server/common/auth/model"
)

func TestIsInGroupNormalCases(t *testing.T) {
	userID := model.ToUserID("dummy@example.com")
	groupName := "group"
	testcases := []struct {
		groupName string
		groups    map[string]*model.AuthGroup
		expected  bool
	}{
		{ // nobody in group.
			groups: map[string]*model.AuthGroup{
				groupName: {},
			},
			expected: false,
		},
		{ // matched in Members.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Members: []string{userID},
				},
			},
			expected: true,
		},
		{ // matched in Globs.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Globs: []string{userID},
				},
			},
			expected: true,
		},
		{ // matched in Globs using globbing.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Globs: []string{model.ToUserID("*@example.com")},
				},
			},
			expected: true,
		},
		{ // matched as nested group member.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Nested: []string{"nested"},
				},
				"nested": {
					Members: []string{userID},
				},
			},
			expected: true,
		},
		{ // matched as nested nested group member.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Nested: []string{"nested"},
				},
				"nested": {
					Nested: []string{"nestednested"},
				},
				"nestednested": {
					Members: []string{userID},
				},
			},
			expected: true,
		},
		{ // matched even if not first Member.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Members: []string{model.ToUserID("another@example.net"), userID},
				},
			},
			expected: true,
		},
		{ // matched even if not first Glob.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Globs: []string{model.ToUserID("*@example.net"), model.ToUserID("*@example.com")},
				},
			},
			expected: true,
		},
		{ // matched even if not in first Nested.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Nested: []string{"dummy", "nested"},
				},
				"dummy": {},
				"nested": {
					Members: []string{userID},
				},
			},
			expected: true,
		},
		{ // should be true for "*" group name.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Nested: []string{"*"},
				},
			},
			expected: true,
		},
	}
	for _, tc := range testcases {
		seen := make(map[string]bool)
		actual := isInGroup(groupName, userID, tc.groups, seen)
		if actual != tc.expected {
			t.Errorf("isInGroup(%q, %q, %q, _) = %v; want %v", groupName, userID, tc.groups, actual, tc.expected)
		}
	}
}

func TestIsInGroupErrorCasesShouldBeIgnored(t *testing.T) {
	userID := model.ToUserID("dummy@example.com")
	groupName := "group"
	testcases := []struct {
		groups map[string]*model.AuthGroup
	}{
		{ // cyclic dependency.
			groups: map[string]*model.AuthGroup{
				groupName: {
					Nested: []string{groupName},
				},
			},
		},
		{ // group name does not exist.
			groups: map[string]*model.AuthGroup{
				"nonexist": {},
			},
		},
	}
	for _, tc := range testcases {
		seen := make(map[string]bool)
		actual := isInGroup(groupName, userID, tc.groups, seen)
		if actual {
			t.Errorf("isInGroup(%q, %q, %q, _) = %v; want false", groupName, userID, tc.groups, actual)
		}
	}
}

func TestFlattenGroupsShouldWorkForNestedGroups(t *testing.T) {
	userID := model.ToUserID("dummy@example.com")
	groups := map[string]*model.AuthGroup{
		"orig": {
			Nested: []string{"nested"},
		},
		"nested": {
			Nested: []string{"nestednested"},
		},
		"nestednested": {
			Members: []string{userID},
		},
		"yetanother": {
			// yet another group that use "nested".
			// if pruning is broken, either this or orig won't work.
			Nested: []string{"nested"},
		},
	}
	fgs := flattenGroups(groups)
	names := []string{"orig", "nested", "nestednested", "yetanother"}
	for _, name := range names {
		actual := fgs[name].has(userID)
		if !actual {
			t.Errorf("fgs[%q]=%v; want true", name, actual)
		}
	}
}
