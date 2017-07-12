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

package utils

import (
	"fmt"
	"sort"
	"strings"
)

// KV is key and value strings.
type KV struct {
	Key   string
	Value string
}

// KVList if list of KV pairs.
type KVList []KV

// ValidateKVList makes sure each string in the list is valid key-value pair.
func ValidateKVList(kind string, list []string, sep rune) error {
	for _, item := range list {
		if !strings.ContainsRune(item, sep) {
			return fmt.Errorf("bad %s, not a 'key%svalue' pair: %q", kind, string(sep), item)
		}
	}
	return nil
}

// KVListFromMap converts a map to KVList.
func KVListFromMap(m map[string]string) KVList {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kvList := make([]KV, len(keys))
	for i, k := range keys {
		kvList[i] = KV{
			Key:   k,
			Value: m[k],
		}
	}
	return kvList
}

// UnpackKVList takes validated list of k-v pair strings and returns list of
// structs.
//
// Silently skips malformed strings. Use ValidateKVList to detect them before
// calling this function.
func UnpackKVList(list []string, sep rune) (out KVList) {
	for _, item := range list {
		idx := strings.IndexRune(item, sep)
		if idx == -1 {
			continue
		}
		out = append(out, KV{
			Key:   item[:idx],
			Value: item[idx+1:],
		})
	}
	return out
}

// Pack converts KV list to a list of strings.
func (l KVList) Pack(sep rune) []string {
	out := make([]string, len(l))
	for i, kv := range l {
		out[i] = kv.Key + string(sep) + kv.Value
	}
	return out
}
