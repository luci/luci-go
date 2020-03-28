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

package job

import (
	"reflect"
	"sort"

	api "go.chromium.org/luci/swarming/proto/api"
)

func keysOf(mapish interface{}) []string {
	mapV := reflect.ValueOf(mapish)
	if mapV.Kind() != reflect.Map {
		panic("keysOf expected a map")
	}
	keys := []string{}
	for _, key := range mapV.MapKeys() {
		if key.Kind() != reflect.String {
			panic("keysOf expected a map with string keys")
		}
		keys = append(keys, key.String())
	}
	sort.Strings(keys)
	return keys
}

func updateStringPairList(list *[]*api.StringPair, updates map[string]string) {
	if len(updates) == 0 {
		return
	}

	current := make(map[string]string, len(*list))
	for _, pair := range *list {
		current[pair.Key] = pair.Value
	}
	for key, value := range updates {
		if value == "" {
			delete(current, key)
		} else {
			current[key] = value
		}
	}
	newList := make([]*api.StringPair, 0, len(current))
	for key, value := range current {
		newList = append(newList, &api.StringPair{Key: key, Value: value})
	}
	*list = newList
}
