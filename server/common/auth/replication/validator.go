// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package replication

import (
	"fmt"
	"path/filepath"
)

// isValidGlob returns true if the given pattern is valid.
func isValidGlob(pattern string) bool {
	_, err := filepath.Match(pattern, pattern)
	return err == nil
}

func validateAuthGroup(groupName string, groups map[string]*AuthGroup, seen, checked map[string]bool) error {
	if checked[groupName] {
		return nil
	}
	if seen[groupName] {
		return fmt.Errorf("detected circular dependency for %v", groupName)
	}
	seen[groupName] = true
	g, ok := groups[groupName]
	if !ok {
		return fmt.Errorf("group %v not found", groupName)
	}

	for _, gb := range g.GetGlobs() {
		if !isValidGlob(gb) {
			return fmt.Errorf("invalid glob %v in %v", gb, groupName)
		}
	}
	for _, n := range g.GetNested() {
		if err := validateAuthGroup(n, groups, seen, checked); err != nil {
			return fmt.Errorf("%v in %v", err, groupName)
		}
	}
	checked[groupName] = true
	return nil
}

func validateAuthGroups(groups map[string]*AuthGroup) error {
	// checked is used for pruning.
	// mark checked if a group is validated.
	checked := make(map[string]bool)
	for n := range groups {
		// seen is used for circular dependency detection.
		seen := make(map[string]bool)
		if err := validateAuthGroup(n, groups, seen, checked); err != nil {
			return err
		}
	}
	return nil
}

// ValidateAuthDB returns nil if valid AuthDB is given.
func ValidateAuthDB(ad *AuthDB) error {
	// Groups
	groups := make(map[string]*AuthGroup)
	for _, g := range ad.GetGroups() {
		groups[g.GetName()] = g
	}
	return validateAuthGroups(groups)
}
