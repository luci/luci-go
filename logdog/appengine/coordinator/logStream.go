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
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"google.golang.org/grpc/codes"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/timestamp"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// CurrentSchemaVersion is the current schema version of the LogStream.
// Changes that are not backward-compatible should update this field so
// migration logic and scripts can translate appropriately.
const CurrentSchemaVersion = "1"

// ErrPathNotFound is the canonical error returned when a Log Stream Path is not found.
var ErrPathNotFound = grpcutil.Errf(codes.NotFound, "path not found")

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
type LogStream struct {
	// ID is the LogStream ID. It is generated from the stream's Prefix/Name
	// fields.
	ID HashID `gae:"$id"`

	// Schema is the datastore schema version for this object. This can be used
	// to facilitate schema migrations.
	//
	// The current schema is currentSchemaVersion.
	Schema string

	// Prefix is this log stream's prefix value. Log streams with the same prefix
	// are logically grouped.
	//
	// This value should not be changed once populated, as it will invalidate the
	// ID.
	Prefix string
	// Name is the unique name of this log stream within the Prefix scope.
	//
	// This value should not be changed once populated, as it will invalidate the
	// ID.
	Name string

	// Created is the time when this stream was created.
	Created time.Time

	// Purged, if true, indicates that this log stream has been marked as purged.
	// Non-administrative queries and requests for this stream will operate as
	// if this entry doesn't exist.
	Purged bool
	// PurgedTime is the time when this stream was purged.
	PurgedTime time.Time `gae:",noindex"`

	// ProtoVersion is the version string of the protobuf, as reported by the
	// Collector (and ultimately self-identified by the Butler).
	ProtoVersion string
	// Descriptor is the binary protobuf data LogStreamDescriptor.
	Descriptor []byte `gae:",noindex"`
	// ContentType is the MIME-style content type string for this stream.
	ContentType string
	// StreamType is the data type of the stream.
	StreamType logpb.StreamType
	// Timestamp is the Descriptor's recorded client-side timestamp.
	Timestamp time.Time

	// Tags is a set of arbitrary key/value tags associated with this stream. Tags
	// can be queried against.
	//
	// The serialization/deserialization is handled manually in order to enable
	// key/value queries.
	Tags TagMap `gae:"-"`

	// extra causes datastore to ignore unrecognized fields and strip them in
	// future writes.
	extra ds.PropertyMap `gae:"-,extra"`

	// noDSValidate is a testing parameter to instruct the LogStream not to
	// validate before reading/writing to datastore. It can be controlled by
	// calling SetDSValidate().
	noDSValidate bool
}

var _ interface {
	ds.PropertyLoadSaver
} = (*LogStream)(nil)

// LogStreamID returns the HashID for a given log stream path.
func LogStreamID(path types.StreamPath) HashID {
	return makeHashID(string(path))
}

// LogPrefix returns a keyed (but not loaded) LogPrefix struct for this
// LogStream's Prefix.
func (s *LogStream) LogPrefix() *LogPrefix {
	return &LogPrefix{ID: s.ID}
}

// PopulateState populates the datastore key fields for the supplied
// LogStreamState, binding them to the current LogStream.
func (s *LogStream) PopulateState(c context.Context, lst *LogStreamState) {
	lst.Parent = ds.KeyForObj(c, s)
}

// State returns the LogStreamState keyed for this LogStream.
func (s *LogStream) State(c context.Context) *LogStreamState {
	var lst LogStreamState
	s.PopulateState(c, &lst)
	return &lst
}

// Path returns the LogDog path for this log stream.
func (s *LogStream) Path() types.StreamPath {
	return types.StreamName(s.Prefix).Join(types.StreamName(s.Name))
}

// Load implements ds.PropertyLoadSaver.
func (s *LogStream) Load(pmap ds.PropertyMap) error {
	// Handle custom properties. Consume them before using the default
	// PropertyLoadSaver.
	for k := range pmap {
		if !strings.HasPrefix(k, "_") {
			continue
		}

		switch k {
		case "_Tags":
			// Load the tag map. Ignore errors.
			tm, _ := tagMapFromProperties(pmap.Slice(k))
			s.Tags = tm
		}
	}

	if err := ds.GetPLS(s).Load(pmap); err != nil {
		return err
	}

	// Validate the log stream. Don't enforce ID correctness, since
	// datastore hasn't populated that field yet.
	if !s.noDSValidate {
		if err := s.validateImpl(false); err != nil {
			return err
		}
	}
	return nil
}

// Save implements ds.PropertyLoadSaver.
func (s *LogStream) Save(withMeta bool) (ds.PropertyMap, error) {
	if !s.noDSValidate {
		if err := s.validateImpl(true); err != nil {
			return nil, err
		}
	}
	s.Schema = CurrentSchemaVersion

	// Save default struct fields.
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

	return pmap, nil
}

// Validate evaluates the state and data contents of the LogStream and returns
// an error if it is invalid.
func (s *LogStream) Validate() error {
	return s.validateImpl(true)
}

func (s *LogStream) validateImpl(enforceHashID bool) error {
	if enforceHashID {
		// Make sure our Prefix and Name match the Hash ID.
		if hid := LogStreamID(s.Path()); hid != s.ID {
			return fmt.Errorf("hash IDs don't match (%q != %q)", hid, s.ID)
		}
	}

	if err := types.StreamName(s.Prefix).Validate(); err != nil {
		return fmt.Errorf("invalid prefix: %s", err)
	}
	if err := types.StreamName(s.Name).Validate(); err != nil {
		return fmt.Errorf("invalid name: %s", err)
	}
	if s.ContentType == "" {
		return errors.New("empty content type")
	}
	if s.Created.IsZero() {
		return errors.New("created time is not set")
	}

	switch s.StreamType {
	case logpb.StreamType_TEXT, logpb.StreamType_BINARY, logpb.StreamType_DATAGRAM:
		break

	default:
		return fmt.Errorf("unsupported stream type: %v", s.StreamType)
	}

	for k, v := range s.Tags {
		if err := types.ValidateTag(k, v); err != nil {
			return fmt.Errorf("invalid tag [%s]: %s", k, err)
		}
	}

	// Ensure that our Descriptor can be unmarshalled.
	if _, err := s.DescriptorValue(); err != nil {
		return fmt.Errorf("could not unmarshal descriptor: %v", err)
	}
	return nil
}

// DescriptorValue returns the unmarshalled Descriptor field protobuf.
func (s *LogStream) DescriptorValue() (*logpb.LogStreamDescriptor, error) {
	pb := logpb.LogStreamDescriptor{}
	if err := proto.Unmarshal(s.Descriptor, &pb); err != nil {
		return nil, err
	}
	return &pb, nil
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
		return fmt.Errorf("invalid descriptor: %v", err)
	}

	pb, err := proto.Marshal(desc)
	if err != nil {
		return fmt.Errorf("failed to marshal descriptor: %v", err)
	}

	s.Prefix = desc.Prefix
	s.Name = desc.Name
	s.ContentType = desc.ContentType
	s.StreamType = desc.StreamType
	s.Descriptor = pb

	// We know that the timestamp is valid b/c it's checked in ValidateDescriptor.
	if ts := desc.Timestamp; ts != nil {
		s.Timestamp = ds.RoundTime(google.TimeFromProto(ts).UTC())
	}

	// Note: tag content was validated via ValidateDescriptor.
	s.Tags = TagMap(desc.Tags)
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

// SetDSValidate controls whether this LogStream is validated prior to being
// read from or written to datastore.
//
// This is a testing parameter, and should NOT be used in production code.
func (s *LogStream) SetDSValidate(v bool) {
	s.noDSValidate = !v
}

// generatePathComponents generates the "_C" property path components for path
// glob querying.
//
// See the comment on LogStream for more information.
func generatePathComponents(prefix, name string) ds.PropertySlice {
	ps, ns := types.StreamName(prefix).Segments(), types.StreamName(name).Segments()

	// Allocate our components array. For each component, there are two entries
	// (forward and reverse), as well as one count entry per component type.
	c := make(ds.PropertySlice, 0, (len(ps)+len(ns)+1)*2)

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

// LogStreamQuery is a function returning `true` if the provided LogStream
// matches.
type LogStreamQuery struct {
	q             *ds.Query
	includePurged bool
	checks        []func(*LogStream) bool
}

// NewLogStreamQuery returns a new LogStreamQuery constrained to the prefix of
// `pathGlob`, and with a filter function for the stream name in `pathGlob`.
//
// By default, it will exclude purged logs.
//
// pathGlob must have a prefix without wildcards, and a stream name portion
// which can include `*` or `**` in any combination.
//
// Returns an error if the supplied pathGlob string describes an invalid query.
func NewLogStreamQuery(pathGlob string) (*LogStreamQuery, error) {
	prefix, name := types.StreamPath(pathGlob).Split()

	if prefix == "" {
		return nil, errors.New("prefix invalid: empty")
	}
	if strings.ContainsRune(string(prefix), '*') {
		return nil, errors.New("prefix invalid: contains wildcard `*`")
	}
	if err := prefix.Validate(); err != nil {
		return nil, errors.Annotate(err, "prefix invalid").Err()
	}

	if name == "" {
		name = "**"
	}
	if err := types.StreamName(strings.Replace(string(name), "*", "a", -1)).Validate(); err != nil {
		return nil, errors.Annotate(err, "name invalid").Err()
	}

	ret := &LogStreamQuery{
		q: ds.NewQuery("LogStream").Eq("Prefix", string(prefix)).Order("-Created"),
	}

	// Escape all regexp metachars. This will have the effect of escaping * as
	// well. We can then replace sequences of escaped *'s to get the expression we
	// want.
	nameEscaped := regexp.QuoteMeta(string(name))
	exp := strings.NewReplacer(
		"/\\*\\*/", "(.*)/",
		"/\\*\\*", "(.*)",
		"\\*\\*/", "(.*)",
		"\\*\\*", "(.*)",
		"\\*", "([^/][^/]*)",
	).Replace(nameEscaped)

	re, err := regexp.Compile(fmt.Sprintf("^%s$", exp))
	if err != nil {
		return nil, errors.Annotate(err, "compiling name regex").Err()
	}

	// this function implements the check for purged as well as the name
	// assertion.
	ret.checks = append(ret.checks, func(ls *LogStream) bool {
		if !ret.includePurged && ls.Purged {
			return false
		}
		return re.MatchString(ls.Name)
	})

	return ret, nil
}

// SetCursor causes the LogStreamQuery to start from the given encoded cursor.
func (lsp *LogStreamQuery) SetCursor(ctx context.Context, cursor string) error {
	if cursor == "" {
		return nil
	}

	cursorObj, err := ds.DecodeCursor(ctx, cursor)
	if err != nil {
		return err
	}

	lsp.q = lsp.q.Start(cursorObj)
	return nil
}

// OnlyContentType constrains the LogStreamQuery to only return LogStreams of
// the given content type.
func (lsp *LogStreamQuery) OnlyContentType(ctype string) {
	if ctype == "" {
		return
	}
	lsp.checks = append(lsp.checks, func(ls *LogStream) bool {
		return ls.ContentType == ctype
	})
}

// OnlyStreamType constrains the LogStreamQuery to only return LogStreams of
// the given stream type.
func (lsp *LogStreamQuery) OnlyStreamType(stype logpb.StreamType) error {
	if _, ok := logpb.StreamType_name[int32(stype)]; !ok {
		return errors.New("unknown StreamType")
	}
	lsp.checks = append(lsp.checks, func(ls *LogStream) bool {
		return ls.StreamType == stype
	})
	return nil
}

// IncludePurged will have the LogStreamQuery return purged logs as well.
func (lsp *LogStreamQuery) IncludePurged() {
	lsp.includePurged = true
}

// OnlyPurged will have the LogStreamQuery return ONLY purged logs.
//
// Will result in NO logs if IncludePurged hasn't been set.
func (lsp *LogStreamQuery) OnlyPurged() {
	lsp.checks = append(lsp.checks, func(ls *LogStream) bool {
		return ls.Purged
	})
}

// TimeBound constrains LogStreams returned to be bound by the given lower and
// upper creation timestamps.
func (lsp *LogStreamQuery) TimeBound(lower, upper *timestamp.Timestamp) {
	// we use a datastore filter here because lsp.q is already ordered by
	// -Created, and so we can apply an inequality to it.
	if lower != nil {
		lsp.q = lsp.q.Gt("Created", google.TimeFromProto(lower).UTC())
	}
	if upper != nil {
		lsp.q = lsp.q.Lt("Created", google.TimeFromProto(upper).UTC())
	}
}

// MustHaveTags constrains LogStreams returned to have all of the given tags.
func (lsp *LogStreamQuery) MustHaveTags(tags map[string]string) {
	lsp.checks = append(lsp.checks, func(ls *LogStream) bool {
		for k, v := range tags {
			actual, ok := ls.Tags[k]
			if !ok {
				return false
			}
			if v != "" && v != actual {
				return false
			}
		}
		return true
	})
}

func (lsp *LogStreamQuery) filter(ls *LogStream) bool {
	for _, checkFn := range lsp.checks {
		if !checkFn(ls) {
			return false
		}
	}
	return true
}

// Run executes the LogStreamQuery and calls `cb` with each LogStream which
// matches the LogStreamQuery.
//
// If `cb` returns ds.Stop, the query will stop with a nil error.
// If `cb` returns a different error, the query will stop with the returned
// error.
// If `cb` returns nil, the query continues until it exhausts.
func (lsp *LogStreamQuery) Run(ctx context.Context, cb func(*LogStream, ds.CursorCB) error) error {
	return ds.Run(ctx, lsp.q, func(ls *LogStream, getCursor ds.CursorCB) (err error) {
		if lsp.filter(ls) {
			err = cb(ls, getCursor)
		}
		return
	})
}
