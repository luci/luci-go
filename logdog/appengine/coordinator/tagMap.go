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

package coordinator

import (
	"encoding/json"
	"fmt"
	"strings"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/common/types"
)

// TagMap is tag map that stores log stream tags into the datastore.
//
// This is serialized to a non-indexed JSON {string:string} object in the
// datastore.
type TagMap map[string]string

var _ ds.PropertyConverter = (*TagMap)(nil)

// ToProperty implements ds.PropertyConverter
func (tm *TagMap) ToProperty() (ds.Property, error) {
	if *tm == nil {
		return ds.MkPropertyNI(nil), nil
	}
	dat, err := json.Marshal(*tm)
	if err != nil {
		panic("impossible: marshalling map[string]string failed.")
	}
	return ds.MkPropertyNI(dat), nil
}

// FromProperty implements ds.PropertyConverter
func (tm *TagMap) FromProperty(prop ds.Property) error {
	if typ := prop.Type(); typ != ds.PTBytes {
		return errors.Reason("wrong type: %s != PTBytes", typ).Err()
	}
	newTM := TagMap{}
	if err := json.Unmarshal(prop.Value().([]byte), &newTM); err != nil {
		return err
	}
	if len(newTM) == 0 {
		*tm = nil
	} else {
		*tm = newTM
	}
	return nil
}

// tagMapFromProperties converts a set of tag property objects into a TagMap.
//
// If an error occurs decoding a specific property, an errors.MultiError will be
// returned alongside the successfully-decoded tags.
func tagMapFromProperties(props ds.PropertySlice) (TagMap, error) {
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
			// Deprecated: when _Tags was indexed, this was used to indicate presence
			// of a key. This case is left in to allow old values to be processed
			// correctly.
			continue
		}
		k, v := parts[0], parts[1]

		if err := types.ValidateTag(k, v); err != nil {
			lme.Assign(idx, fmt.Errorf("invalid tag %q: %s", parts[0], err))
			continue
		}
		tm[k] = v
	}

	if len(tm) == 0 {
		tm = nil
	}
	return tm, lme.Get()
}
