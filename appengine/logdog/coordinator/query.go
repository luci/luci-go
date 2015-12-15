// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"fmt"
	"strings"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
)

// QueryBase is a datastore model containing queryable information about a log
// stream. It is intended to be embedded in various query-supporting structs.
// Each such struct can choose its own key to enable various types of sorting.
//
// This structure has additional datastore fields imposed by the
// PropertyLoadSaver. These fields enable querying against some of the complex
// data types:
//   - _Prefix.count is the number of segments in the Prefix.
//   - _Prefix.<idx> is the stream name <idx>th segment in the Prefix.
//   - _Prefix.r.<idx> is the stream name <idx>th segment in the Prefix from the
//     end.
//   - _Name.count is the number of segments in the Name.
//   - _Name.<idx> is the stream name <idx>th segment in the Name.
//   - _Name.r.<idx> is the stream name <idx>th segment in the Name from the
//      end.
//   - _Tag is a string slice containing:
//     - KEY=[VALUE] key/value tags.
//     - KEY key presence tags.
//
// Most of the values in QueryBase are static. Those that change can only be
// changed through service endpoint methods.
//
// LogStream's QueryBase is authortative.
type QueryBase struct {
	// Prefix is this log stream's prefix value. Log streams with the same prefix
	// are logically grouped.
	Prefix string
	// Name is the unique name of this log stream within the Prefix scope.
	Name string

	// ContentType is the MIME-style content type string for this stream.
	ContentType string
	// StreamType is the data type of the stream.
	StreamType protocol.LogStreamDescriptor_StreamType

	// Source is the set of source strings sent by the Butler.
	Source []string

	// ProtoVersion is the version string of the protobuf, as reported by the
	// Collector (and ultimately self-identified by the Butler).
	ProtoVersion string

	// Tags is a set of arbitrary key/value tags associated with this stream. Tags
	// can be queried against.
	//
	// The serialization/deserialization is handled manually in order to enable
	// key/value queries.
	Tags TagMap `gae:"-"`

	// Terminated is true if the log stream has been terminated.
	Terminated bool
	// Archived is true if the log stream has been archived.
	Archived bool

	// Purged, if true, indicates that this log stream has been marked as purged.
	// Non-administrative queries and requests for this stream will operate as
	// if this entry doesn't exist.
	Purged bool
}

// Path returns the LogDog path for this log stream.
func (s *QueryBase) Path() types.StreamPath {
	return types.StreamName(s.Prefix).Join(types.StreamName(s.Name))
}

// PLSLoadFrom implements datastore.PropertyLoadSaver Load method. It must be
// called by embedding objects during their Load method to load query values.
func (s *QueryBase) PLSLoadFrom(pmap ds.PropertyMap) error {
	// Handle custom properties. Consume them before using the default
	// PropertyLoadSaver.
	for k, v := range pmap {
		if !strings.HasPrefix(k, "_") {
			continue
		}

		switch k {
		case "_Tags":
			// Load the tag map. Ignore errors.
			tm, _ := tagMapFromProperties(v)
			s.Tags = tm
		}
		delete(pmap, k)
	}
	return nil
}

// PLSSaveTo implements ds.PropertyLoadSaver Save method. It must be called by
// embedding objects during their Save method to store query values.
func (s *QueryBase) PLSSaveTo(pmap ds.PropertyMap, withMeta bool) error {
	err := error(nil)
	pmap["_Tags"], err = s.Tags.toProperties()
	if err != nil {
		return fmt.Errorf("failed to encode tags: %v", err)
	}

	// Project our path components ("Prefix", "Name").
	addComponent := func(base string, value types.StreamName) {
		segments := value.Segments()
		pmap[fmt.Sprintf("%s.count", base)] = []ds.Property{ds.MkProperty(len(segments))}

		// Forward mapping (a/b/c => 0:a, 1:b, 2:c).
		for i, seg := range segments {
			pmap[fmt.Sprintf("%s.%d", base, i)] = []ds.Property{ds.MkProperty(seg)}
		}

		// Reverse mapping (a/b/c => 0:c, 1:b, 2:c).
		for i := range segments {
			pmap[fmt.Sprintf("%s.r.%d", base, i)] = []ds.Property{ds.MkProperty(segments[len(segments)-i-1])}
		}
	}
	addComponent("_Prefix", types.StreamName(s.Prefix))
	addComponent("_Name", types.StreamName(s.Name))
	return nil
}

func addComponentFilter(q *ds.Query, base, field, rfield string, value types.StreamName) (*ds.Query, error) {
	segments := value.Segments()
	if len(segments) == 0 {
		// All-component query; impose no constraints.
		return q, nil
	}

	// Profile the string. If it doesn't have any glob characters in it,
	// fully constrain the base field.
	hasGlob := false
	for i, seg := range segments {
		switch seg {
		case "*", "**":
			hasGlob = true

		default:
			// Regular segment. Assert that it is valid.
			if err := types.StreamName(seg).Validate(); err != nil {
				return nil, fmt.Errorf("invalid %s component at index %d (%s): %s", base, i, seg, err)
			}
		}
	}
	if !hasGlob {
		// Direct field (base) query.
		return q.Eq(base, string(value)), nil
	}

	// Add specific field constraints for each non-glob segment.
	greedy := false
	rstack := []string(nil)
	for i, seg := range segments {
		switch seg {
		case "*":
			// Skip asserting this segment.
			if greedy {
				// Add a placeholder (e.g., .../**/a/*/b, placeholder ensures "a" gets
				// position -2 instead of -1.
				//
				// Note that "" can never be a segment, because we validate each
				// non-glob path segment and "" is not a valid stream name component.
				rstack = append(rstack, "")
			}
			continue

		case "**":
			if greedy {
				return nil, fmt.Errorf("cannot have more than one greedy glob")
			}

			// Mark that we're greedy, and skip asserting this segment.
			greedy = true
			continue

		default:
			if greedy {
				// Add this to our reverse stack. We'll query the reverse field from
				// this stack after we know how many elements are ultimately in it.
				rstack = append(rstack, seg)
			} else {
				q = q.Eq(fmt.Sprintf("%s.%d", field, i), seg)
			}
		}
	}

	// Add the reverse stack to pin the elements at the end of the path name
	// (e.g., a/b/**/c/d, stack will be {c, d}, need to map to {r.0=d, r.1=c}.
	for i, seg := range rstack {
		if seg == "" {
			// Placeholder, skip.
			continue
		}
		q = q.Eq(fmt.Sprintf("%s.%d", rfield, len(rstack)-i-1), seg)
	}

	// If we're not greedy, fix this size of this component.
	if !greedy {
		q = q.Eq(fmt.Sprintf("%s.count", field), len(segments))
	}
	return q, nil
}

// AddLogStreamPathQuery constructs a compiled LogStreamPathQuery. It will
// return an error if the supllied query string describes an invalid query.
func AddLogStreamPathQuery(q *ds.Query, path string) (*ds.Query, error) {
	prefix, name := types.StreamPath(path).Split()

	err := error(nil)
	q, err = addComponentFilter(q, "Prefix", "_Prefix", "_Prefix.r", prefix)
	if err != nil {
		return nil, err
	}
	q, err = addComponentFilter(q, "Name", "_Name", "_Name.r", name)
	if err != nil {
		return nil, err
	}
	return q, nil
}
