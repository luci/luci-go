// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/lazyslot"
	"github.com/luci/luci-go/common/logging"

	"github.com/luci/luci-go/server/auth/service/protocol"
	"github.com/luci/luci-go/server/middleware"
)

// ErrNoDB is returned by default DB returned from GetDB if no DBFactory is
// installed in the context.
var ErrNoDB = errors.New("auth: using default auth.DB, install a properly mocked one instead")

// DB is interface to access a database of authorization related information.
//
// It is static read only object that represent snapshot of auth data at some
// moment in time.
type DB interface {
	// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
	// to authenticate access for given email.
	IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error)
}

// DBFactory returns most recent DB instance.
//
// The factory is injected into the context by UseDB and used by GetDB.
type DBFactory func(c context.Context) (DB, error)

// DBCacheUpdater knows how to update local in-memory copy of DB.
//
// Used by NewDBCache.
type DBCacheUpdater func(c context.Context, prev DB) (DB, error)

// dbKey is used for context.Context key of DBFactory.
type dbKey int

// UseDB sets a factory that creates DB instances.
func UseDB(c context.Context, f DBFactory) context.Context {
	return context.WithValue(c, dbKey(0), f)
}

// WithDB is middleware that sets given DBFactory in the context before calling
// a handler.
func WithDB(h middleware.Handler, f DBFactory) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(UseDB(c, f), rw, r, p)
	}
}

// GetDB returns most recent snapshot of authorization database using factory
// installed in the context via `UseDB`.
//
// If no factory is installed, returns DB that forbids everything and logs
// errors. It is often good enough for unit tests that do not care about
// authorization, and still not horribly bad if accidentally used in production.
//
// Requiring each unit test to install fake DB factory is a bit cumbersome.
func GetDB(c context.Context) (DB, error) {
	if f, _ := c.Value(dbKey(0)).(DBFactory); f != nil {
		return f(c)
	}
	return ErroringDB{Error: ErrNoDB}, nil
}

// NewDBCache returns a factory of DB instances that uses local memory to
// cache DB instances for 5 seconds. It uses supplied callback to refetch DB
// from some permanent storage when cache expires.
//
// Even though the return value is technically a function, treat it as a heavy
// stateful object, since it has the cache of DB in its closure.
func NewDBCache(updater DBCacheUpdater) DBFactory {
	cacheSlot := lazyslot.Slot{
		Fetcher: func(c context.Context, prev lazyslot.Value) (lazyslot.Value, error) {
			var prevDB DB
			if prev.Value != nil {
				prevDB = prev.Value.(DB)
			}
			newDB, err := updater(c, prevDB)
			if err != nil {
				return lazyslot.Value{}, err
			}
			return lazyslot.Value{
				Value:      newDB,
				Expiration: clock.Now(c).Add(5 * time.Second),
			}, nil
		},
	}

	// Actual factory that just grabs DB from the cache (triggering lazy refetch).
	return func(c context.Context) (DB, error) {
		val, err := cacheSlot.Get(c)
		if err != nil {
			return nil, err
		}
		return val.Value.(DB), nil
	}
}

// ErroringDB implements DB by forbidding all access and returning errors.
//
// See explanation in GetDB.
type ErroringDB struct {
	Error error // returned by all calls
}

// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
// to authenticate access for given email.
func (db ErroringDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	logging.Errorf(c, "%s", db.Error)
	return false, db.Error
}

///

// OAuth client_id of https://apis-explorer.appspot.com/.
const googleAPIExplorerClientID = "292824132082.apps.googleusercontent.com"

// SnapshotDB implements DB using AuthDB proto message.
//
// Use NewSnapshotDB to create new instances. Don't touch public fields
// of existing instances.
type SnapshotDB struct {
	AuthServiceURL string // where it was fetched from
	Rev            int64  // its revision number

	clientIDs map[string]struct{} // set of allowed client IDs
}

var _ DB = &SnapshotDB{}

// NewSnapshotDB creates new instance of SnapshotDB.
//
// It does some preprocessing to speed up subsequent checks.
func NewSnapshotDB(authDB *protocol.AuthDB, authServiceURL string, rev int64) *SnapshotDB {
	db := &SnapshotDB{
		AuthServiceURL: authServiceURL,
		Rev:            rev,
		clientIDs:      make(map[string]struct{}, 2+len(authDB.GetOauthAdditionalClientIds())),
	}

	// Set of all allowed clientIDs.
	db.clientIDs[googleAPIExplorerClientID] = struct{}{}
	if authDB.GetOauthClientId() != "" {
		db.clientIDs[authDB.GetOauthClientId()] = struct{}{}
	}
	for _, cid := range authDB.GetOauthAdditionalClientIds() {
		if cid != "" {
			db.clientIDs[cid] = struct{}{}
		}
	}

	return db
}

// IsAllowedOAuthClientID returns true if given OAuth2 client_id can be used
// to authenticate access for given email.
func (db *SnapshotDB) IsAllowedOAuthClientID(c context.Context, email, clientID string) (bool, error) {
	// No need to whitelist client IDs for service accounts, since email address
	// uniquely identifies credentials used. Note: this is Google specific.
	if strings.HasSuffix(email, ".gserviceaccount.com") {
		return true, nil
	}

	// clientID must be set for non service accounts.
	if clientID == "" {
		return false, nil
	}

	_, ok := db.clientIDs[clientID]
	return ok, nil
}
