// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

import (
	"reflect"
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

// byWhitelistKeyID is used for sorting IPWhitelists by entity key's IntID().
type byWhitelistKeyID []AuthIPWhitelist

func (b byWhitelistKeyID) Len() int      { return len(b) }
func (b byWhitelistKeyID) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byWhitelistKeyID) Less(i, j int) bool {
	return b[i].Key.IntID() < b[j].Key.IntID()
}

// currentAuthDBSnapshot returns current database's AuthDBSnapshot.
// This function should be called under transaction context.
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
			sort.Sort(byWhitelistKeyID(snap.IPWhitelists))
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

// entity represents a datastore entity.
type entity interface {
	// key() result is used by datastore.PutMulti().
	key() *datastore.Key
}

// inGroups returns true if exact matching g is in groups.
func inGroups(g AuthGroup, groups []AuthGroup) bool {
	for _, gitr := range groups {
		if reflect.DeepEqual(g, gitr) {
			return true
		}
	}
	return false
}

// groupsDiff is used to get difference of two []AuthGroup.
// newEntities contains the new data or updated data.
// deleteKeys represents the keys to delete.
func groupsDiff(curgrps, newgrps []AuthGroup) (newEntities []entity, deleteKeys []*datastore.Key) {
	exists := make(map[string]bool)
	for _, n := range newgrps {
		exists[n.Key.Encode()] = true
		if !inGroups(n, curgrps) {
			newEntities = append(newEntities, &n)
		}
	}
	for _, cg := range curgrps {
		if !exists[cg.Key.Encode()] {
			deleteKeys = append(deleteKeys, cg.Key)
		}
	}
	return newEntities, deleteKeys
}

// inWhitelists returns true if exact matching a is in wls.
func inWhitelists(a AuthIPWhitelist, wls []AuthIPWhitelist) bool {
	for _, wl := range wls {
		if reflect.DeepEqual(a, wl) {
			return true
		}
	}
	return false
}

// whitelistsDiff is used to get difference of two []AuthWhitelist.
// newEntities contains the new data or updated data.
// deleteKeys represents the keys to delete.
func whitelistsDiff(curwls, newwls []AuthIPWhitelist) (newEntities []entity, deleteKeys []*datastore.Key) {
	exists := make(map[string]bool)
	for _, n := range newwls {
		exists[n.Key.Encode()] = true
		if !inWhitelists(n, curwls) {
			newEntities = append(newEntities, &n)
		}
	}
	for _, cw := range curwls {
		if !exists[cw.Key.Encode()] {
			deleteKeys = append(deleteKeys, cw.Key)
		}
	}
	return newEntities, deleteKeys
}

// ReplaceAuthDB updates database with given AuthDBSnapshot if it is new.
// The first return value indicates database was updated,
// and the second one has the latest AuthReplicationState.
func ReplaceAuthDB(c context.Context, newData AuthDBSnapshot) (bool, *AuthReplicationState, error) {
	var stat *AuthReplicationState
	updated := false

	err := datastore.RunInTransaction(c, func(c context.Context) error {
		curstat := &AuthReplicationState{}
		if err := GetReplicationState(c, curstat); err != nil {
			return err
		}
		// database is already up-to-date.
		if curstat.AuthDBRev >= newData.ReplicationState.AuthDBRev {
			stat = curstat
			return nil
		}

		dbsnap := &AuthDBSnapshot{}
		if err := currentAuthDBSnapshot(c, dbsnap); err != nil {
			return err
		}

		var newEntities []entity
		var delKeys []*datastore.Key
		// Going to update database.
		if !reflect.DeepEqual(newData.GlobalConfig, dbsnap.GlobalConfig) {
			newEntities = append(newEntities, &newData.GlobalConfig)
		}

		newGrps, delGrKeys := groupsDiff(dbsnap.Groups, newData.Groups)
		newEntities = append(newEntities, newGrps...)
		delKeys = append(delKeys, delGrKeys...)

		newWls, delWlKeys := whitelistsDiff(dbsnap.IPWhitelists, newData.IPWhitelists)
		newEntities = append(newEntities, newWls...)
		delKeys = append(delKeys, delWlKeys...)

		if !reflect.DeepEqual(newData.IPWhitelistAssignments, dbsnap.IPWhitelistAssignments) {
			newEntities = append(newEntities, &newData.IPWhitelistAssignments)
		}

		curstat.AuthDBRev = newData.ReplicationState.AuthDBRev
		curstat.ModifiedTimestamp = newData.ReplicationState.ModifiedTimestamp

		var wg sync.WaitGroup
		ch := make(chan error, 3)
		wg.Add(3)
		go func() {
			defer wg.Done()
			if _, err := datastore.Put(c, ReplicationStateKey(c), curstat); err != nil {
				ch <- err
				return
			}
			ch <- nil
		}()
		go func() {
			defer wg.Done()
			keys := make([]*datastore.Key, 0, len(newEntities))
			for _, n := range newEntities {
				keys = append(keys, n.key())
			}
			if _, err := datastore.PutMulti(c, keys, newEntities); err != nil {
				ch <- err
				return
			}
			ch <- nil
		}()
		go func() {
			defer wg.Done()
			if err := datastore.DeleteMulti(c, delKeys); err != nil {
				ch <- err
				return
			}
			ch <- nil
		}()
		go func() {
			wg.Wait()
			close(ch)
		}()

		for err := range ch {
			if err != nil {
				return err
			}
		}
		stat = curstat
		updated = true
		return nil
	}, &datastore.TransactionOptions{
		XG: true,
	})
	if err != nil {
		return false, nil, err
	}
	return updated, stat, nil
}

// TODO(yyanagisawa): write unittest.
//                    Since there is no test framework for
//                    google.golang.org/appengine, we need to implement
//                    test doubles by ourselves.
