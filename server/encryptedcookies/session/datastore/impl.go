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

	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"
)

// Store uses Cloud Datastore for sessions.
type Store struct {
	Namespace string // the datastore namespace to use or "" for default
}

var _ session.Store = (*Store)(nil)

// FetchSession fetches an existing session with the given ID.
//
// Returns (nil, nil) if there's no such session.
func (s *Store) FetchSession(ctx context.Context, id session.ID) (*sessionpb.Session, error) {
	ctx = info.MustNamespace(ctx, s.Namespace)
	ent := sessionEntity{ID: entityID(id)}
	switch err := datastore.Get(ctx, &ent); {
	case err == datastore.ErrNoSuchEntity:
		return nil, nil
	case err != nil:
		return nil, err
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
// retried. Errors from callbacks are returned as is.
func (s *Store) UpdateSession(ctx context.Context, id session.ID, cb func(*sessionpb.Session) error) error {
	ctx = info.MustNamespace(ctx, s.Namespace)
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var mutable *sessionpb.Session
		ent := sessionEntity{ID: entityID(id)}
		switch err := datastore.Get(ctx, &ent); {
		case err == datastore.ErrNoSuchEntity:
			mutable = &sessionpb.Session{}
		case err != nil:
			return err
		default:
			mutable = ent.Session
		}
		if err := cb(mutable); err != nil {
			return err
		}
		return datastore.Put(ctx, makeEntity(id, mutable))
	}, nil)
}

type sessionEntity struct {
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
}

func entityID(id session.ID) string {
	return base64.RawStdEncoding.EncodeToString(id)
}

func makeEntity(id session.ID, s *sessionpb.Session) *sessionEntity {
	return &sessionEntity{
		ID:          entityID(id),
		Session:     s,
		State:       s.State,
		Created:     s.Created.AsTime(),
		LastRefresh: s.LastRefresh.AsTime(),
		NextRefresh: s.NextRefresh.AsTime(),
		Closed:      s.Closed.AsTime(),
		Sub:         s.Sub,
		Email:       s.Email,
	}
}
