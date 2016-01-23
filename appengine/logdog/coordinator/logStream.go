// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// LogStreamState is the archival state of the log stream.
type LogStreamState int

const (
	// LSPending indicates that no archival has occurred yet.
	LSPending LogStreamState = iota
	// LSTerminated indicates that the log stream has received a terminal index
	// and is awaiting archival.
	LSTerminated
	// LSArchived indicates that the log stream has been successfully archived but
	// has not yet been cleaned up.
	LSArchived
	// LSDone indicates that log stream processing is complete.
	LSDone
)

// LogStream is the primary datastore model containing information and state of
// an individual log stream.
//
// This structure contains the standard queryable fields, and is the source of
// truth for log stream state. Writes to LogStream should be done via Put, which
// will ensure that the LogStream's related query objects are kept in sync.
//
// This structure has additional datastore fields imposed by the
// PropertyLoadSaver. These fields enable querying against some of the complex
// data types:
//	  - _C breaks the the Prefix and Name fields into positionally-queryable
//	    entries. It is used to build globbing queries.
//
//	    It is composed of entries detailing (T is the path [P]refix or [N]ame):
//	    - TF:n:value ("T" has a component, "value", at index "n").
//	    - TR:n:value ("T" has a component, "value", at reverse-index "n").
//	    - TC:count ("T" has "count" total elements).
//
//	    For example, the path "foo/bar/+/baz" would break into:
//	    ["PF:0:foo", "PF:1:bar", "PR:0:bar", "PR:1:foo", "PC:2", "NF:0:baz",
//	     "NR:0:baz", "NC:1"].
//
//	  - _Tags is a string slice containing:
//	    - KEY=[VALUE] key/value tags.
//	    - KEY key presence tags.
//
//	  - _Terminated is true if the LogStream has been terminated.
//	  - _Archived is true if the LogStream has been archived.
//
// Most of the values in QueryBase are static. Those that change can only be
// changed through service endpoint methods.
//
// LogStream's QueryBase is authortative.
type LogStream struct {
	// Prefix is this log stream's prefix value. Log streams with the same prefix
	// are logically grouped.
	Prefix string
	// Name is the unique name of this log stream within the Prefix scope.
	Name string

	// State is the log stream's current state.
	State LogStreamState

	// Purged, if true, indicates that this log stream has been marked as purged.
	// Non-administrative queries and requests for this stream will operate as
	// if this entry doesn't exist.
	Purged bool

	// Secret is the Butler secret value for this stream.
	//
	// This value may only be returned to LogDog services; it is not user-visible.
	Secret []byte `gae:",noindex"`

	// Created is the time when this stream was created.
	Created time.Time
	// Updated is the Coordinator's record of when this log stream was last
	// updated.
	Updated time.Time

	// ProtoVersion is the version string of the protobuf, as reported by the
	// Collector (and ultimately self-identified by the Butler).
	ProtoVersion string
	// Descriptor is the binary protobuf data LogStreamDescriptor.
	Descriptor []byte `gae:",noindex"`
	// ContentType is the MIME-style content type string for this stream.
	ContentType string
	// StreamType is the data type of the stream.
	StreamType logpb.LogStreamDescriptor_StreamType
	// Timestamp is the Descriptor's recorded client-side timestamp.
	Timestamp time.Time

	// Tags is a set of arbitrary key/value tags associated with this stream. Tags
	// can be queried against.
	//
	// The serialization/deserialization is handled manually in order to enable
	// key/value queries.
	Tags TagMap `gae:"-"`

	// Source is the set of source strings sent by the Butler.
	Source []string

	// TerminalIndex is the log stream index of the last log entry in the stream.
	// If the value is -1, the log is still streaming.
	TerminalIndex int64 `gae:",noindex"`

	// ArchiveIndexURL is the Google Storage URL where the log stream's index is
	// archived.
	ArchiveIndexURL string `gae:",noindex"`
	// ArchiveStreamURL is the Google Storage URL where the log stream's raw
	// stream data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveStreamURL string `gae:",noindex"`
	// ArchiveDataURL is the Google Storage URL where the log stream's assembled
	// data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveDataURL string `gae:",noindex"`

	// _ causes datastore to ignore unrecognized fields and strip them in future
	// writes.
	_ ds.PropertyMap `gae:"-,extra"`

	// hashID is the cached generated ID from the stream's Prefix/Name fields. If
	// this is populated, ID metadata will be retrieved from this field instead of
	// generated.
	hashID string
}

var _ interface {
	ds.PropertyLoadSaver
	ds.MetaGetterSetter
} = (*LogStream)(nil)

// NewLogStream returns a LogStream instance with its ID field initialized based
// on the supplied path.
//
// The supplied value is a LogDog stream path or a hash of the LogDog stream
// path.
func NewLogStream(value string) (*LogStream, error) {
	path := types.StreamPath(value)
	if err := path.Validate(); err != nil {
		// If it's not a path, see if it's a SHA256 sum.
		hash, hashErr := normalizeHash(value)
		if hashErr != nil {
			return nil, fmt.Errorf("invalid path (%s) and hash (%s)", err, hashErr)
		}

		// Load this LogStream with its SHA256 hash directly. This stream will not
		// have its Prefix/Name fields populated until it's loaded from datastore.
		return LogStreamFromID(hash), nil
	}

	// It is a valid path.
	return LogStreamFromPath(path), nil
}

// LogStreamFromID returns an empty LogStream instance with a known hash ID.
func LogStreamFromID(hashID string) *LogStream {
	return &LogStream{
		hashID: hashID,
	}
}

// LogStreamFromPath returns an empty LogStream instance initialized from a
// known path value.
//
// The supplied path is assumed to be valid and is not checked.
func LogStreamFromPath(path types.StreamPath) *LogStream {
	// Load the prefix/name fields into the log stream.
	prefix, name := path.Split()
	return &LogStream{
		Prefix: string(prefix),
		Name:   string(name),
	}
}

// Path returns the LogDog path for this log stream.
func (s *LogStream) Path() types.StreamPath {
	return types.StreamName(s.Prefix).Join(types.StreamName(s.Name))
}

// Load implements ds.PropertyLoadSaver.
func (s *LogStream) Load(pmap ds.PropertyMap) error {
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

	return ds.GetPLS(s).Load(pmap)
}

// Save implements ds.PropertyLoadSaver.
func (s *LogStream) Save(withMeta bool) (ds.PropertyMap, error) {
	pmap, err := ds.GetPLS(s).Save(withMeta)
	if err != nil {
		return nil, err
	}

	// Encode _Tags.
	pmap["_Tags"], err = s.Tags.toProperties()
	if err != nil {
		return nil, fmt.Errorf("failed to encode tags: %v", err)
	}

	// Generate our path components, "_C".
	pmap["_C"] = generatePathComponents(s.Prefix, s.Name)

	// Add our derived statuses.
	pmap["_Terminated"] = []ds.Property{ds.MkProperty(s.Terminated())}
	pmap["_Archived"] = []ds.Property{ds.MkProperty(s.Archived())}

	return pmap, nil
}

// GetMeta implements ds.MetaGetterSetter.
func (s *LogStream) GetMeta(key string) (interface{}, bool) {
	switch key {
	case "id":
		return s.HashID(), true

	default:
		return ds.GetPLS(s).GetMeta(key)
	}
}

// GetAllMeta implements ds.MetaGetterSetter.
func (s *LogStream) GetAllMeta() ds.PropertyMap {
	pmap := ds.GetPLS(s).GetAllMeta()
	pmap.SetMeta("id", ds.MkProperty(s.HashID()))
	return pmap
}

// SetMeta implements ds.MetaGetterSetter.
func (s *LogStream) SetMeta(key string, val interface{}) bool {
	return ds.GetPLS(s).SetMeta(key, val)
}

// HashID generates and populates the hashID field of a LogStream. This
// is the hash of the log stream's full path.
func (s *LogStream) HashID() string {
	if s.hashID == "" {
		if s.Prefix == "" || s.Name == "" {
			panic("cannot generate ID hash: Prefix and Name are not populated")
		}

		hash := sha256.Sum256([]byte(s.Path()))
		s.hashID = hex.EncodeToString(hash[:])
	}
	return s.hashID
}

// Put writes this LogStream to the Datastore. Before writing, it validates that
// LogStream is complete.
func (s *LogStream) Put(di ds.Interface) error {
	if err := s.Validate(); err != nil {
		return err
	}
	return di.Put(s)
}

// Validate evaluates the state and data contents of the LogStream and returns
// an error if it is invalid.
func (s *LogStream) Validate() error {
	if err := types.StreamName(s.Prefix).Validate(); err != nil {
		return fmt.Errorf("invalid prefix: %s", err)
	}
	if err := types.StreamName(s.Name).Validate(); err != nil {
		return fmt.Errorf("invalid name: %s", err)
	}
	if len(s.Secret) != types.StreamSecretLength {
		return fmt.Errorf("invalid secret length (%d != %d)", len(s.Secret), types.StreamSecretLength)
	}
	if s.ContentType == "" {
		return errors.New("empty content type")
	}
	if s.Created.IsZero() {
		return errors.New("created time is not set")
	}
	if s.Updated.IsZero() {
		return errors.New("updated time is not set")
	}
	if s.Updated.Before(s.Created) {
		return fmt.Errorf("updated time must be >= created time (%s < %s)", s.Updated, s.Created)
	}

	switch s.StreamType {
	case logpb.LogStreamDescriptor_TEXT, logpb.LogStreamDescriptor_BINARY,
		logpb.LogStreamDescriptor_DATAGRAM:
		break

	default:
		return fmt.Errorf("unsupported stream type: %v", s.StreamType)
	}

	for k, v := range s.Tags {
		tag := types.StreamTag{Key: k, Value: v}
		if err := tag.Validate(); err != nil {
			return fmt.Errorf("invalid tag [%s]: %s", k, err)
		}
	}
	if _, err := s.DescriptorProto(); err != nil {
		return fmt.Errorf("invalid descriptor protobuf: %s", err)
	}
	return nil
}

// Terminated returns true if this stream has been terminated.
func (s *LogStream) Terminated() bool {
	return s.State >= LSTerminated
}

// Archived returns true if this stream has been archived. A stream is archived
// if it has any of its archival properties set.
func (s *LogStream) Archived() bool {
	return s.State >= LSArchived
}

// ArchiveMatches tests if the supplied Stream, Index, and Data archival URLs
// match the current values.
func (s *LogStream) ArchiveMatches(sURL, iURL, dURL string) bool {
	return (s.ArchiveStreamURL == sURL && s.ArchiveIndexURL == iURL && s.ArchiveDataURL == dURL)
}

// LoadDescriptor loads the fields in the log stream descriptor into this
// LogStream entry. These fields are:
//   - Prefix
//   - Name
//   - ContentType
//   - StreamType
//   - Descriptor
//   - Timestamp
//   - Tags
func (s *LogStream) LoadDescriptor(desc *logpb.LogStreamDescriptor) error {
	if err := desc.Validate(true); err != nil {
		return fmt.Errorf("invalid descriptor: %s", err)
	}

	// Marshal the descriptor.
	data, err := proto.Marshal(desc)
	if err != nil {
		return fmt.Errorf("failed to marshal descriptor: %s", err)
	}

	s.Prefix = desc.Prefix
	s.Name = desc.Name
	s.ContentType = desc.ContentType
	s.StreamType = desc.StreamType
	s.Descriptor = data

	// We know that the timestamp is valid b/c it's checked in ValidateDescriptor.
	if ts := desc.Timestamp; ts != nil {
		s.Timestamp = NormalizeTime(ts.Time().UTC())
	}

	// Note: tag content was validated via ValidateDescriptor.
	if tags := desc.Tags; len(tags) > 0 {
		s.Tags = make(TagMap, len(tags))
		for _, tag := range tags {
			s.Tags[tag.Key] = tag.Value
		}
	}
	return nil
}

// DescriptorProto unmarshals a LogStreamDescriptor from the stream's Descriptor
// field. It will return an error if the unmarshalling fails.
func (s *LogStream) DescriptorProto() (*logpb.LogStreamDescriptor, error) {
	desc := logpb.LogStreamDescriptor{}
	if err := proto.Unmarshal(s.Descriptor, &desc); err != nil {
		return nil, err
	}
	return &desc, nil
}

// normalizeHash takes a SHA256 hexadecimal string as input. It validates that
// it is a valid SHA256 hash and, if so, returns a normalized version that can
// be used as a log stream key.
func normalizeHash(v string) (string, error) {
	if decodeSize := hex.DecodedLen(len(v)); decodeSize != sha256.Size {
		return "", fmt.Errorf("invalid SHA256 hash size (%d != %d)", decodeSize, sha256.Size)
	}
	b, err := hex.DecodeString(v)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// generatePathComponents generates the "_C" property path components for path
// glob querying.
//
// See the comment on LogStream for more infromation.
func generatePathComponents(prefix, name string) []ds.Property {
	ps, ns := types.StreamName(prefix).Segments(), types.StreamName(name).Segments()

	// Allocate our components array. For each component, there are two entries
	// (forward and reverse), as well as one count entry per component type.
	c := make([]ds.Property, 0, (len(ps)+len(ns)+1)*2)

	gen := func(b string, segs []string) {
		// Generate count component (PC:4).
		c = append(c, ds.MkProperty(fmt.Sprintf("%sC:%d", b, len(segs))))

		// Generate forward and reverse components.
		for i, s := range segs {
			c = append(c,
				// Forward (PF:0:foo).
				ds.MkProperty(fmt.Sprintf("%sF:%d:%s", b, i, s)),
				// Reverse (PR:3:foo)
				ds.MkProperty(fmt.Sprintf("%sR:%d:%s", b, len(segs)-i-1, s)),
			)
		}
	}
	gen("P", ps)
	gen("N", ns)
	return c
}

func addComponentFilter(q *ds.Query, full, f, base string, value types.StreamName) (*ds.Query, error) {
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
				return nil, fmt.Errorf("invalid %s component at index %d (%s): %s", full, i, seg, err)
			}
		}
	}
	if !hasGlob {
		// Direct field (full) query.
		return q.Eq(full, string(value)), nil
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
				q = q.Eq(f, fmt.Sprintf("%sF:%d:%s", base, i, seg))
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
		q = q.Eq(f, fmt.Sprintf("%sR:%d:%s", base, len(rstack)-i-1, seg))
	}

	// If we're not greedy, fix this size of this component.
	if !greedy {
		q = q.Eq(f, fmt.Sprintf("%sC:%d", base, len(segments)))
	}
	return q, nil
}

// AddLogStreamPathFilter constructs a compiled LogStreamPathQuery. It will
// return an error if the supllied query string describes an invalid query.
func AddLogStreamPathFilter(q *ds.Query, path string) (*ds.Query, error) {
	prefix, name := types.StreamPath(path).Split()

	err := error(nil)
	q, err = addComponentFilter(q, "Prefix", "_C", "P", prefix)
	if err != nil {
		return nil, err
	}
	q, err = addComponentFilter(q, "Name", "_C", "N", name)
	if err != nil {
		return nil, err
	}
	return q, nil
}

// AddLogStreamTerminatedFilter returns a derived query that asserts that a log
// stream has been terminated.
func AddLogStreamTerminatedFilter(q *ds.Query, v bool) *ds.Query {
	return q.Eq("_Terminated", v)
}

// AddLogStreamArchivedFilter returns a derived query that asserts that a log
// stream has been archived.
func AddLogStreamArchivedFilter(q *ds.Query, v bool) *ds.Query {
	return q.Eq("_Archived", v)
}

// AddLogStreamPurgedFilter returns a derived query that asserts that a log
// stream has been archived.
func AddLogStreamPurgedFilter(q *ds.Query, v bool) *ds.Query {
	return q.Eq("Purged", v)
}

// AddOlderFilter adds a filter to queries that restricts them to results that
// were created before the supplied time.
func AddOlderFilter(q *ds.Query, t time.Time) *ds.Query {
	return q.Lt("Created", t.UTC()).Order("-Created")
}

// AddNewerFilter adds a filter to queries that restricts them to results that
// were created after the supplied time.
func AddNewerFilter(q *ds.Query, t time.Time) *ds.Query {
	return q.Gt("Created", t.UTC()).Order("-Created")
}
