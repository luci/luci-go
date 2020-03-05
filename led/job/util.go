// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
