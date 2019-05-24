// Copyright 2018 The LUCI Authors.
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

package graph

import (
	"fmt"
	"strings"
)

// KeySet is a set of all keys ever defined in a graph.
//
// Each key is a singleton object: asking for the same key twice returns exact
// same *Key object. This simplifies comparison of keys and using keys as, well,
// keys in a map.
type KeySet struct {
	keys map[string]*Key // compact representation of the key path -> *Key
}

// Key returns a *Key given a list of (kind, id) pairs.
//
// There can be at most one kind that starts with '@...' (aka "namespace kind"),
// and it must come first (if present).
func (k *KeySet) Key(pairs ...string) (*Key, error) {
	switch {
	case len(pairs) == 0:
		return nil, fmt.Errorf("empty key path")
	case len(pairs)%2 != 0:
		return nil, fmt.Errorf("key path %q has odd number of components", pairs)
	}

	for _, s := range pairs {
		if strings.IndexByte(s, 0) != -1 {
			return nil, fmt.Errorf("bad key path element %q, has zero byte inside", s)
		}
	}

	for i := 2; i < len(pairs); i += 2 {
		if strings.HasPrefix(pairs[i], "@") {
			return nil, fmt.Errorf("kind %q can appear only at the start of the key path", pairs[i])
		}
	}

	keyID := strings.Join(pairs, "\x00")
	if key := k.keys[keyID]; key != nil {
		return key, nil
	}

	if k.keys == nil {
		k.keys = make(map[string]*Key, 1)
	}

	key := &Key{set: k, pairs: pairs, idx: len(k.keys), cmp: keyID}
	k.keys[keyID] = key
	return key, nil
}
