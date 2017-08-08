// Copyright 2015 The LUCI Authors.
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
	"encoding/json"
	"sync"
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/clock"
)

// MemoryStorage implements Storage interface, using memory as a backend. Useful
// in unit tests.
type MemoryStorage struct {
	Expiration time.Duration // default expiration time of in-memory cache

	lock   sync.Mutex
	values map[string]*json.RawMessage
}

// FetchAllSettings fetches all latest settings at once.
func (m *MemoryStorage) FetchAllSettings(c context.Context) (*Bundle, error) {
	m.lock.Lock()
	defer m.lock.Unlock()
	values := make(map[string]*json.RawMessage, len(m.values))
	for k, v := range m.values {
		if v == nil {
			values[k] = v
		} else {
			raw := append(json.RawMessage(nil), *v...)
			values[k] = &raw
		}
	}
	return &Bundle{Values: values, Exp: clock.Now(c).Add(m.Expiration)}, nil
}

// UpdateSetting updates a setting at the given key.
func (m *MemoryStorage) UpdateSetting(c context.Context, key string, value json.RawMessage, who, why string) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	if m.values == nil {
		m.values = make(map[string]*json.RawMessage, 1)
	}
	cpy := append(json.RawMessage(nil), value...)
	m.values[key] = &cpy
	return nil
}
