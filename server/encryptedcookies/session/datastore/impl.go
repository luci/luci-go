// Copyright 2021 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package datastore implements session storage over Cloud Datastore.
package datastore

import (
	"context"
	"encoding/base64"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/server/encryptedcookies/internal"
	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
	"go.chromium.org/luci/server/gaeemulation"
	"go.chromium.org/luci/server/module"
)

// InactiveSessionExpiration is used to derive ExpireAt field of the datastore
// entity.
//
// It defines how long to keep inactive session in the datastore before they
// are cleaned up by a TTL policy.
const InactiveSessionExpiration time.Duration = 14 * 24 * time.Hour

// Store uses Cloud Datastore for sessions.
type Store struct {
	Namespace string // the datastore namespace to use or "" for default
}

var _ session.Store = (*Store)(nil)

func init() {
	internal.RegisterStoreImpl(internal.StoreImpl{
		ID: "datastore",
		Factory: func(ctx context.Context, namespace string) (session.Store, error) {
			return &Store{Namespace: namespace}, nil
		},
		Deps: []module.Dependency{
			module.RequiredDependency(gaeemulation.ModuleName),
		},
	})
}

// FetchSession fetches an existing session with the given ID.
//
// Returns (nil, nil) if there's no such session. All errors are transient.
func (s *Store) FetchSession(ctx context.Context, id session.ID) (*sessionpb.Session, error) {
	ctx = info.MustNamespace(ctx, s.Namespace)
	ent := SessionEntity{ID: entityID(id)}
	switch err := datastore.Get(ctx, &ent); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, transient.Tag.Apply(err)
	default:
		return ent.Session, nil
	}
}

// UpdateSession transactionally updates or creates a session.
//
// If fetches the session, calls the callback to mutate it, and stores the
// result. If it is a new session, the callback receives an empty proto.
//
// The callback may be called multiple times in case the transaction is
// retried. Errors from callbacks are returned as is. All other errors are
// transient.
func (s *Store) UpdateSession(ctx context.Context, id session.ID, cb func(*sessionpb.Session) error) error {
	ctx = info.MustNamespace(ctx, s.Namespace)
	var cbErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cbErr = nil
		var mutable *sessionpb.Session
		ent := SessionEntity{ID: entityID(id)}
		switch err := datastore.Get(ctx, &ent); {
		case err == datastore.ErrNoSuchEntity:
			mutable = &sessionpb.Session{}
		case err != nil:
			return err
		default:
			mutable = ent.Session
		}
		if cbErr = cb(mutable); cbErr != nil {
			return cbErr
		}
		var lastRefresh time.Time
		if mutable.LastRefresh != nil {
			lastRefresh = mutable.LastRefresh.AsTime()
		} else {
			lastRefresh = clock.Now(ctx).UTC()
		}
		exp := lastRefresh.Add(InactiveSessionExpiration)
		return datastore.Put(ctx, makeEntity(id, mutable, exp))
	}, nil)
	if err == cbErr {
		return cbErr // can also be nil on success
	}
	return transient.Tag.Apply(err)
}

// SessionEntity is what is actually stored in the datastore.
type SessionEntity struct {
	_kind  string                `gae:"$kind,encryptedcookies.Session"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID      string             `gae:"$id"`
	Session *sessionpb.Session `gae:",nocompress"`

	// Fields extracted from Session for indexing.

	State       sessionpb.State
	Created     time.Time
	LastRefresh time.Time
	NextRefresh time.Time
	Closed      time.Time
	Sub         string
	Email       string

	// ExpireAt is used in a Cloud Datastore TTL policy.
	//
	// It is derived from LastRefresh (or the current time if LastRefresh is not
	// populated) based on InactiveSessionExpiration whenever the entity is
	// stored.
	ExpireAt time.Time `gae:",noindex"`
}

func entityID(id session.ID) string {
	return base64.RawStdEncoding.EncodeToString(id)
}

func makeEntity(id session.ID, s *sessionpb.Session, exp time.Time) *SessionEntity {
	return &SessionEntity{
		ID:          entityID(id),
		Session:     s,
		State:       s.State,
		Created:     asTime(s.Created),
		LastRefresh: asTime(s.LastRefresh),
		NextRefresh: asTime(s.NextRefresh),
		Closed:      asTime(s.Closed),
		Sub:         s.Sub,
		Email:       s.Email,
		ExpireAt:    exp,
	}
}

func asTime(t *timestamppb.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.AsTime()
}
