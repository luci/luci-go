// Copyright 2016 The LUCI Authors.
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

package datastorecache

import (
	"strings"

	"github.com/luci/luci-go/appengine/memlock"

	"github.com/luci/gae/service/info"

	"golang.org/x/net/context"
)

// ErrFailedToLock is a sentinel error returned by Locker.TryWithLock if the
// lock is already held by another entity.
var ErrFailedToLock = memlock.ErrFailedToLock

// Locker is an interface to a generic locking function.
type Locker interface {
	// TryWithLock blocks on acquiring a lock for the specified key, invokes the
	// supplied function while holding the lock, and releases the lock before
	// returning.
	//
	// If the lock is already held, TryWithLock should return ErrFailedToLock.
	// Otherwise, TryWithLock will forward the return value of fn.
	TryWithLock(c context.Context, key string, fn func(context.Context) error) error
}

// memLocker is a Locker implementation that uses the memlock library.
type memLocker struct {
	clientID string
}

// MemLocker returns a Locker instance that uses a memcache lock bound to the
// current request ID.
func MemLocker(c context.Context) Locker {
	return &memLocker{
		clientID: strings.Join([]string{
			"datastore_cache",
			info.RequestID(c),
		}, "\x00"),
	}
}

func (m *memLocker) TryWithLock(c context.Context, key string, fn func(context.Context) error) error {
	switch err := memlock.TryWithLock(c, key, m.clientID, fn); err {
	case memlock.ErrFailedToLock:
		return ErrFailedToLock
	default:
		return err
	}
}

// nopLocker is a Locker instance that performs no actual locking.
type nopLocker struct{}

func (nopLocker) TryWithLock(c context.Context, key string, fn func(context.Context) error) error {
	return fn(c)
}
