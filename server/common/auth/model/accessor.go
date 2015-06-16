// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"sort"
	"sync"

	"golang.org/x/net/context"
	"google.golang.org/appengine/datastore"
)

// GetReplicationState returns AuthReplicationState singleton entity.
func GetReplicationState(c context.Context, a *AuthReplicationState) error {
	return datastore.Get(c, ReplicationStateKey(c), a)
}

// AuthDBSnapshot represents a snapshot of AuthDB.
type AuthDBSnapshot struct {
	// ReplicationState represents AuthReplicationState of this snapshot.
	ReplicationState AuthReplicationState
	// GlobalConfig represents AuthGlobalConfig of this snapshot.
	GlobalConfig AuthGlobalConfig
	// Groups represents list of AuthGroup.
	Groups []AuthGroup
	// IPWhitelists represents list of AuthIPWhitelist.
	IPWhitelists []AuthIPWhitelist
	// IPWhitelistsAssignments represents AuthIPWhitelistAssignments.
	IPWhitelistAssignments AuthIPWhitelistAssignments
}

// byWhitelistKeyId is used for sorting IPWhitelists by entity key's IntID().
type byWhitelistKeyId []AuthIPWhitelist

func (b byWhitelistKeyId) Len() int { return len(b) }
func (b byWhitelistKeyId) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byWhitelistKeyId) Less(i, j int) bool {
	return b[i].Key.IntID() < b[j].Key.IntID()
}

// currentAuthDBSnapshot is called under transaction context by
// CurrentAuthDBSnapshot.
// It gets a snapshot of AuthDB to be stored to snap.
func currentAuthDBSnapshot(c context.Context, snap *AuthDBSnapshot) error {
	errCh := make(chan error, 4)

	var wg sync.WaitGroup
	wg.Add(4)
	go func() {
		defer wg.Done()
		errCh <- GetReplicationState(c, &snap.ReplicationState)
	}()
	go func() {
		defer wg.Done()
		errCh <- datastore.Get(c, RootKey(c), &snap.GlobalConfig)
	}()
	go func() {
		defer wg.Done()
		q := datastore.NewQuery("AuthGroup").Ancestor(RootKey(c))
		_, err := q.GetAll(c, &snap.Groups)
		errCh <- err
	}()
	go func() {
		defer wg.Done()
		key := IPWhitelistAssignmentsKey(c)
		err := datastore.Get(c, key, &snap.IPWhitelistAssignments)
		if err != nil {
			if err != datastore.ErrNoSuchEntity {
				errCh <- err
				return
			}
			snap.IPWhitelistAssignments.Key = key
		}
		var names []string
		for _, a := range snap.IPWhitelistAssignments.Assignments {
			names = append(names, a.IPWhitelist)
		}
		names = append(names, "bots")
		type repl struct {
			authIPwl AuthIPWhitelist
			err      error
		}
		ch := make(chan repl, len(names))
		for _, n := range names {
			go func(n string) {
				var a AuthIPWhitelist
				err := datastore.Get(c, IPWhitelistKey(c, n), &a)
				ch <- repl{
					authIPwl: a,
					err:      err,
				}
			}(n)
		}
		for i := 0; i < len(names); i++ {
			r := <-ch
			if r.err == datastore.ErrNoSuchEntity {
				continue
			}
			if r.err != nil {
				errCh <- r.err
				return
			}
			snap.IPWhitelists = append(snap.IPWhitelists, r.authIPwl)
			sort.Sort(byWhitelistKeyId(snap.IPWhitelists))
		}
		errCh <- nil
	}()

	go func() {
		wg.Wait()
		close(errCh)
	}()

	for err := range errCh {
		if err != nil && err != datastore.ErrNoSuchEntity {
			return err
		}
	}
	return nil
}

// CurrentAuthDBSnapshot returns current AuthDBSnapshot.
func CurrentAuthDBSnapshot(c context.Context) (*AuthDBSnapshot, error) {
	snap := &AuthDBSnapshot{}
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		return currentAuthDBSnapshot(c, snap)
	}, nil)
	if err != nil {
		return nil, err
	}
	return snap, err
}

// TODO(yyanagisawa): write unittest.
//                    Since there is no test framework for
//                    google.golang.org/appengine, we need to implement
//                    test doubles by ourselves.
