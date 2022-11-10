// Copyright 2022 The LUCI Authors.
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

package starlarkproto

import (
	"go.starlark.net/starlark"
)

// Key of the MessageCache in starlark.Thread local store.
const threadMessageCache = "starlarkproto.MessageCache"

// MessageCache knows how to cache deserialized messages.
//
// Semantically it is a map `(cache, typ, body) => frozen Message`, where
// `cache` identifies a serialization format, `typ` identifies the type being
// deserialized and `body` is the raw message body to be deserialized.
//
// Works with frozen messages.
//
// Use SetMessageCache to install a cache into a starlark thread.
type MessageCache interface {
	// Fetch returns a previously stored message or (nil, nil) if missing.
	Fetch(th *starlark.Thread, cache, body string, typ *MessageType) (*Message, error)
	// Store stores a deserialized message.
	Store(th *starlark.Thread, cache, body string, msg *Message) error
}

// SetMessageCache installs the given messages cache into the thread.
//
// It will be used by all deserialization operations performed by the Starlark
// code in this thread.
func SetMessageCache(th *starlark.Thread, mc MessageCache) {
	th.SetLocal(threadMessageCache, mc)
}

// messageCache returns the message cache in the thread.
func messageCache(th *starlark.Thread) MessageCache {
	mc, _ := th.Local(threadMessageCache).(MessageCache)
	return mc
}
