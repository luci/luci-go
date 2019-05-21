// Copyright 2019 The LUCI Authors.
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

package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sync/atomic"
	"time"

	"go.chromium.org/luci/common/logging"
)

// ExternalStorage implements Storage interface using an externally supplied
// io.Reader with JSON.
//
// This is read-only storage (it doesn't implement MutableStorage interface),
// meaning settings it stores can't be change via UpdateSetting (or admin UI).
//
// They still can be dynamically reloaded via Load() though.
type ExternalStorage struct {
	values atomic.Value // settings as map[string]*json.RawMessage, immutable
}

// Load loads (or reloads) settings from a given reader.
//
// The reader should supply a JSON document with a dict. Keys are strings
// matching various settings pages, and values are corresponding settings
// (usually also dicts). After settings are loaded (and the context is properly
// configured), calling settings.Get(c, <key>, &output) deserializes the value
// supplied for the corresponding key into 'output'.
//
// On success logs what has changed (if anything) and eventually starts serving
// new settings (see comments for FetchAllSettings regarding the caching).
//
// On failure returns an error without any logging or without changing what is
// currently being served.
func (s *ExternalStorage) Load(c context.Context, r io.Reader) error {
	var newS map[string]*json.RawMessage
	if err := json.NewDecoder(r).Decode(&newS); err != nil {
		return err
	}

	// Diff old and new settings, emit difference into the log. Note that we do it
	// even on the first Load (when old settings are empty), to log initial
	// settings.
	type change struct {
		Old *json.RawMessage `json:"old,omitempty"`
		New *json.RawMessage `json:"new,omitempty"`
	}
	added := map[string]*json.RawMessage{}   // key => new added value
	removed := map[string]*json.RawMessage{} // key => old removed value
	changed := map[string]change{}           // key => old and new values

	oldS, _ := s.values.Load().(map[string]*json.RawMessage)
	addedKeys, removedKeys, sameKeys := diffKeys(newS, oldS)

	for _, k := range addedKeys {
		added[k] = newS[k]
	}
	for _, k := range removedKeys {
		removed[k] = oldS[k]
	}
	for _, k := range sameKeys {
		oldV := oldS[k]
		newV := newS[k]
		if !bytes.Equal(*oldV, *newV) {
			changed[k] = change{Old: oldV, New: newV}
		}
	}

	if len(added) != 0 || len(removed) != 0 || len(changed) != 0 {
		(logging.Fields{
			"added":   added,
			"removed": removed,
			"changed": changed,
		}).Warningf(c, "Settings updated")
	}

	s.values.Store(newS)
	return nil
}

// FetchAllSettings is part of Storage interface, it returns a bundle with all
// latest settings and its expiration time.
//
// The bundle has expiration time of 10 sec, so that Settings{...}
// implementation doesn't go through slower cache-miss code path all the time
// a setting is read (which happens multiple times per request). In practice it
// means changes loaded via Load() apply at most 10 sec later.
func (s *ExternalStorage) FetchAllSettings(context.Context) (*Bundle, time.Duration, error) {
	values, _ := s.values.Load().(map[string]*json.RawMessage)
	return &Bundle{Values: values}, 10 * time.Second, nil
}

// diffKeys returns 3 sets of keys: a-b, b-a and a&b.
func diffKeys(a, b map[string]*json.RawMessage) (aMb, bMa, aAb []string) {
	for k := range a {
		if _, ok := b[k]; ok {
			aAb = append(aAb, k)
		} else {
			aMb = append(aMb, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			bMa = append(bMa, k)
		}
	}
	return
}
