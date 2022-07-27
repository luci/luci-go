// Copyright 2020 The LUCI Authors.
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

// Package db defines common database interface.
package db

import (
	"context"
	"fmt"

	"go.chromium.org/luci/server/module"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

// DB abstracts out specific storage implementation.
type DB interface {
	// Kind identifies this particular database implementation.
	//
	// Among other things it is used in Cloud Tasks messages involved in the
	// implementation of the distributed sweeping.
	Kind() string

	// Defer defers the execution of the callback until the transaction lands.
	//
	// Panics if called outside of a transaction.
	//
	// The callback will receive the original non-transactional context.
	Defer(context.Context, func(context.Context))

	// SaveReminder persists reminder in a transaction context.
	//
	// Tags retriable errors as transient.
	SaveReminder(context.Context, *reminder.Reminder) error

	// DeleteReminder deletes reminder in a non-transaction context.
	DeleteReminder(context.Context, *reminder.Reminder) error

	// FetchRemindersMeta fetches Reminders with Ids in [low..high) range.
	//
	// RawPayload of Reminders should not be fetched.
	// Both fresh & stale reminders should be fetched.
	// The reminders should be returned in order of ascending Id.
	//
	// In case of error, partial result of fetched Reminders so far should be
	// returned alongside the error. The caller will later call this method again
	// to fetch the remaining of Reminders in range of [<lastReturned.Id+1> .. high).
	FetchRemindersMeta(ctx context.Context, low, high string, limit int) ([]*reminder.Reminder, error)

	// FetchReminderRawPayloads fetches raw payloads of a batch of Reminders.
	//
	// The Reminder objects are re-used in the returned batch.
	// If any Reminder is no longer found, it is silently omitted in the returned
	// batch.
	// In case of any other error, partial result of fetched Reminders so far
	// should be returned alongside the error.
	FetchReminderRawPayloads(context.Context, []*reminder.Reminder) ([]*reminder.Reminder, error)
}

// Impl knows how to instantiate DB instances.
type Impl struct {
	// Kind identifies this particular DB implementation.
	//
	// Must match Kind() of the produced DB instance.
	Kind string

	// Module is name of the server module with DB implementation, if any.
	Module module.Name

	// ProbeForTxn "probes" a context for an active transaction, returning a DB
	// that can be used to transactionally submit reminders or nil if this is not
	// a transactional context.
	ProbeForTxn func(context.Context) DB

	// NonTxn returns an instance of DB that can be used outside of
	// transactions.
	//
	// This is used by the sweeper to enumerate reminders.
	NonTxn func(context.Context) DB
}

var impls []Impl

// Register registers a database implementation.
//
// Must be called during init() time.
func Register(db Impl) {
	if db.Kind == "" {
		panic("Kind must not be empty")
	}
	for _, impl := range impls {
		if impl.Kind == db.Kind {
			panic(fmt.Sprintf("DB %q is already registered", db.Kind))
		}
	}
	impls = append(impls, db)
}

// Configured returns true if there's at least one registered implementation.
func Configured() bool {
	return len(impls) > 0
}

// Kinds returns IDs of registered database implementations.
func Kinds() []string {
	kinds := make([]string, len(impls))
	for i, impl := range impls {
		kinds[i] = impl.Kind
	}
	return kinds
}

// VisitImpls calls the callback for all registered implementation.
func VisitImpls(cb func(db *Impl)) {
	for i := range impls {
		cb(&impls[i])
	}
}

// TxnDB returns a Database that matches the context or nil.
//
// The process has a list of database engines registered via Register. Given
// a context, TxnDB examines if it carries a transaction with any of the
// registered DBs.
//
// Panics if more than one database matches the context.
func TxnDB(ctx context.Context) (db DB) {
	for _, impl := range impls {
		if d := impl.ProbeForTxn(ctx); d != nil {
			if db != nil {
				panic(fmt.Sprintf("multiple databases match the context: %q and %q", db.Kind(), d.Kind()))
			}
			db = d
		}
	}
	return
}

// NonTxnDB returns a database with given ID or nil if not registered.
func NonTxnDB(ctx context.Context, id string) DB {
	for _, impl := range impls {
		if impl.Kind == id {
			return impl.NonTxn(ctx)
		}
	}
	return nil
}
