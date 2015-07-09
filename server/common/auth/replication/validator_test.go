// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

import (
	"testing"

	"github.com/luci/luci-go/server/common/auth/model"
)

func TestIsValidGlob(t *testing.T) {
	testcases := []struct {
		pattern  string
		expected bool
	}{
		{
			"pattern",
			true,
		},
		{ // with asterisk
			"*",
			true,
		},
		{ // simple with bracket.
			"[p]attern",
			true,
		},
		{ // asterisk in bracket.
			"[*]attern",
			true,
		},
		{ // simple with bracket range.
			"p[a-t]tern",
			true,
		},
		{ // without close bracket.
			"[p",
			false,
		},
		{ // empty bracket.
			"[]",
			false,
		},
		{ // empty bracket with ^.
			"[^]",
			false,
		},
		{ // bracket with several bracket open and one close
			"[[]",
			true,
		},
		{ // double ^ in bracket is valid.
			"[^^]",
			true,
		},
	}

	for _, tc := range testcases {
		actual := isValidGlob(tc.pattern)
		if actual != tc.expected {
			t.Errorf("isValidGlob(%v)=%v; want %v", tc.pattern, actual, tc.expected)
		}
	}
}

func TestValidateAuthGroupsShouldWork(t *testing.T) {
	groupName := "group"
	userID := "user@example.com"
	testcases := []struct {
		groups map[string]*AuthGroup
	}{
		{ // nobody in group.
			groups: map[string]*AuthGroup{
				groupName: {},
			},
		},
		{ // matched in Members.
			groups: map[string]*AuthGroup{
				groupName: {
					Members: []string{userID},
				},
			},
		},
		{ // matched in Globs.
			groups: map[string]*AuthGroup{
				groupName: {
					Globs: []string{userID},
				},
			},
		},
		{ // matched in Globs using globbing.
			groups: map[string]*AuthGroup{
				groupName: {
					Globs: []string{model.ToUserID("*@example.com")},
				},
			},
		},
		{ // matched as nested group member.
			groups: map[string]*AuthGroup{
				groupName: {
					Nested: []string{"nested"},
				},
				"nested": {
					Members: []string{userID},
				},
			},
		},
		{ // matched as nested nested group member.
			groups: map[string]*AuthGroup{
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
		},
		{ // matched even if not first Member.
			groups: map[string]*AuthGroup{
				groupName: {
					Members: []string{model.ToUserID("another@example.net"), userID},
				},
			},
		},
		{ // matched even if not first Glob.
			groups: map[string]*AuthGroup{
				groupName: {
					Globs: []string{model.ToUserID("*@example.net"), model.ToUserID("*@example.com")},
				},
			},
		},
		{ // matched even if not in first Nested.
			groups: map[string]*AuthGroup{
				groupName: {
					Nested: []string{"dummy", "nested"},
				},
				"dummy": {},
				"nested": {
					Members: []string{userID},
				},
			},
		},
	}
	for _, tc := range testcases {
		err := validateAuthGroups(tc.groups)
		if err != nil {
			t.Errorf("validateAuthGroups(%q) = %v; want <nil>", tc.groups, err)
		}
	}
}

func TestValidateAuthGroupsErrorCases(t *testing.T) {
	groupName := "group"
	testcases := []struct {
		groups map[string]*AuthGroup
	}{
		{ // circular dependency.
			groups: map[string]*AuthGroup{
				groupName: {
					Nested: []string{groupName},
				},
			},
		},
		{ // circular dependency (mutually recursive).
			groups: map[string]*AuthGroup{
				"group1": {
					Nested: []string{"group2"},
				},
				"group2": {
					Nested: []string{"group3"},
				},
				"group3": {
					Nested: []string{"group1"},
				},
			},
		},
		{ // broken glob.
			groups: map[string]*AuthGroup{
				groupName: {
					Globs: []string{"["},
				},
			},
		},
		{ // group name does not exist.
			groups: map[string]*AuthGroup{
				groupName: {
					Nested: []string{"notexist"},
				},
			},
		},
	}
	for _, tc := range testcases {
		err := validateAuthGroups(tc.groups)
		if err == nil {
			t.Errorf("validateAuthGroups(%q) = <nil>; want error", tc.groups)
		}
	}
}
