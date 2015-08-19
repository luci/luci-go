// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package api

import (
	"errors"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/appengine/log"

	"github.com/luci/luci-go/server/common/auth/model"
)

var (
	cache *authCacheService
)
var (
	ErrAlreadyInitialized = errors.New("auth cache: already initialized")
	ErrNotInitialized     = errors.New("auth cache: not initialized")
	ErrUnknownGroup       = errors.New("auth cache: unknown group")
)

const updateInterval = time.Hour

type flatGroup struct {
	// TODO(yyanagisawa): consider use of regexp for luci-py compatibility.
	//                    [^seq] in go is [!seq] in python.
	globs   []string
	members map[string]bool
}

func flattenGroup(groupName string, groups map[string]*model.AuthGroup, flatGroups map[string]*flatGroup, seen map[string]bool) {
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
	flatGroups[groupName] = &flatGroup{
		globs:   gs,
		members: members,
	}
}

func flattenGroups(groups map[string]*model.AuthGroup) map[string]*flatGroup {
	flatGroups := make(map[string]*flatGroup)
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

type authCacheService struct {
	mu     sync.RWMutex
	groups map[string]*flatGroup
}

func latestFlatGroups(c context.Context) (map[string]*flatGroup, error) {
	snap := &model.AuthDBSnapshot{}
	if err := model.CurrentAuthDBSnapshot(c, snap); err != nil {
		return nil, err
	}
	ag := make(map[string]*model.AuthGroup, len(snap.Groups))
	for _, g := range snap.Groups {
		ag[g.Key.StringID()] = g
	}
	return flattenGroups(ag), nil
}

func (a *authCacheService) setGroups(g map[string]*flatGroup) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.groups = g
}

func (a *authCacheService) group(name string) (*flatGroup, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.groups == nil {
		return nil, ErrNotInitialized
	}
	return a.groups[name], nil
}

func (a *authCacheService) isInitialized() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.groups != nil
}

func (a *authCacheService) update(c context.Context) {
	fgs, err := latestFlatGroups(c)
	if err != nil {
		log.Errorf(c, "failed to get latest authgroup: %v", err)
		return
	}
	a.setGroups(fgs)
}

func (a *authCacheService) run(c context.Context, t time.Ticker) {
	for range t.C {
		a.update(c)
	}
}

// Initialize initializes cache service for auth.
//
// c should be appengine.BackgroundContext().
func Initialize(c context.Context, t time.Ticker) error {
	if cache != nil {
		log.Errorf(c, "auth cache already initialized")
		return ErrAlreadyInitialized
	}

	cache = &authCacheService{}
	cache.update(c)
	go cache.run(c, t)
	if !cache.isInitialized() {
		return ErrNotInitialized
	}
	return nil
}

// IsGroupMember returns true if a member is in a group.
//
// Note that this function ignores errors caused by broken AuthGroup.
// e.g. globs field is broken or cyclic dependency.
// Otherwise broken AuthGroup can kill IsGroupMember function.
//
// Also, not to accept bad glob, all AuthDB proto instances must be verified
// by ValidateAuthDB in github.com/luci/luci-go/server/common/auth/replication
// before stored to datastore.
func IsGroupMember(c context.Context, groupName, userName string) (bool, error) {
	if cache == nil {
		return false, ErrNotInitialized
	}

	fg, err := cache.group(groupName)
	if err != nil {
		return false, err
	}
	if fg == nil {
		return false, ErrUnknownGroup
	}
	return fg.has(userName), nil
}
