// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"fmt"
	"sort"
	"strings"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logdog/types"
)

// TagMap is tag map that stores log stream tags into the datastore.
//
// Tags are stored both as presence entries (Key) and as equality entries
// (Key=Value). Both entry contents are encoded via encodeKey.
type TagMap map[string]string

// tagMapFromProperties converts a set of tag property objects into a TagMap.
//
// If an error occurs decoding a specific property, an errors.MultiError will be
// returned alongside the successfully-decoded tags.
func tagMapFromProperties(props []ds.Property) (TagMap, error) {
	tm := TagMap{}
	lme := errors.NewLazyMultiError(len(props))
	for idx, prop := range props {
		v, ok := prop.Value().(string)
		if !ok {
			lme.Assign(idx, fmt.Errorf("property is not a string (%T)", prop.Value()))
			continue
		}
		e, err := decodeKey(v)
		if err != nil {
			lme.Assign(idx, fmt.Errorf("failed to decode property (%q): %s", v, err))
			continue
		}

		parts := strings.SplitN(e, "=", 2)
		if len(parts) != 2 {
			// This is a presence entry. Ignore.
			continue
		}

		st := types.StreamTag{
			Key:   parts[0],
			Value: parts[1],
		}
		if err := st.Validate(); err != nil {
			lme.Assign(idx, fmt.Errorf("invalid tag (%#v): %s", st, err))
			continue
		}
		tm[st.Key] = st.Value
	}

	if len(tm) == 0 {
		tm = nil
	}
	return tm, lme.Get()
}

// toProperties converts a TagMap to a set of Property objects for storage.
func (m TagMap) toProperties() ([]ds.Property, error) {
	if len(m) == 0 {
		return nil, nil
	}

	// Deterministic conversion.
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]ds.Property, 0, len(m)*2)
	for _, k := range keys {
		st := types.StreamTag{
			Key:   k,
			Value: m[k],
		}
		if err := st.Validate(); err != nil {
			return nil, err
		}

		// Presence entry.
		parts = append(parts, ds.MkProperty(encodeKey(st.Key)))

		// Value entry.
		parts = append(parts, ds.MkProperty(encodeKey(fmt.Sprintf("%s=%s", st.Key, st.Value))))
	}
	return parts, nil
}

// AddLogStreamTagFilter adds a tag filter to a Query object.
//
// This method will only add equality filters to the query. If value is empty,
// a presence filter will be added; otherwise, an equality filter will be added.
//
// This incorporates the encoding expressed by TagMap.
func AddLogStreamTagFilter(q *ds.Query, key string, value string) *ds.Query {
	if value == "" {
		return q.Eq("_Tags", encodeKey(key))
	}
	return q.Eq("_Tags", encodeKey(fmt.Sprintf("%s=%s", key, value)))
}
