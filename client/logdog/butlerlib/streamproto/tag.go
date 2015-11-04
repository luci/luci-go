// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamproto

import (
	"encoding/json"
	"flag"
	"sort"

	"github.com/luci/luci-go/client/internal/flags/stringmapflag"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
)

// TagMap is a flags-compatible map used to store stream tags.
type TagMap stringmapflag.Value

var _ interface {
	json.Marshaler
	json.Unmarshaler
	flag.Value
} = (*TagMap)(nil)

// String implements flag.Value.
func (t *TagMap) String() string {
	return (*stringmapflag.Value)(t).String()
}

// Set implements flag.Value
func (t *TagMap) Set(key string) error {
	return (*stringmapflag.Value)(t).Set(key)
}

// SortedKeys returns a sorted slice of the keys in a TagMap.
func (t TagMap) SortedKeys() []string {
	if len(t) == 0 {
		return nil
	}

	keys := make([]string, 0, len(t))
	for k := range t {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Proto returns a LogStreamDescriptor_Tag protobuf entry populated with the
// contents of the TagMap.
func (t TagMap) Proto() []*protocol.LogStreamDescriptor_Tag {
	if len(t) == 0 {
		return nil
	}

	keys := t.SortedKeys()
	tags := make([]*protocol.LogStreamDescriptor_Tag, len(keys))
	for i, k := range keys {
		tags[i] = &protocol.LogStreamDescriptor_Tag{
			Key:   k,
			Value: t[k],
		}
	}
	return tags
}

// MarshalJSON implements the json.Marshaler interface.
func (t *TagMap) MarshalJSON() ([]byte, error) {
	m := make(map[string]string, len(*t))
	if len(*t) > 0 {
		for k, v := range *t {
			m[k] = v
		}
	}
	return json.Marshal(m)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *TagMap) UnmarshalJSON(data []byte) error {
	m := map[string]string{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	if len(m) == 0 {
		*t = nil
		return nil
	}

	tm := make(TagMap, len(m))
	for k, v := range m {
		if err := validateTagMapEntry(k, v); err != nil {
			return err
		}
		tm[k] = v
	}

	*t = tm
	return nil
}

func validateTagMapEntry(key, value string) error {
	return (&types.StreamTag{
		Key:   key,
		Value: value,
	}).Validate()
}
