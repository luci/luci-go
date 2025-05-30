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
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

// CurrentSchemaVersion is the current schema version of the LogStream.
// Changes that are not backward-compatible should update this field so
// migration logic and scripts can translate appropriately.
//
// History:
//
//	1 - Contained _Tags and _C queryable fields
//	2 - Removed _Tags and _C queryable fields and applied noindex to
//	    most fields, since query filtering is now implemented in-memory instead
//	    of via datastore filters.
//	3 - Removed all non-indexed fields which are redundant with content in
//	    Descriptor.
const CurrentSchemaVersion = "3"

// ErrPathNotFound is the canonical error returned when a Log Stream Path is not found.
var ErrPathNotFound = status.Error(codes.NotFound, "path not found")

// LogStreamExpiry is the duration after creation that a LogStream
// record should persist for.  After this duration it may be deleted.
const LogStreamExpiry = 540 * 24 * time.Hour

// LogStream is the primary datastore model containing information and state of
// an individual log stream.
type LogStream struct {
	// ID is the LogStream ID. It is generated from the stream's Prefix/Name
	// fields.
	ID HashID `gae:"$id"`

	// Schema is the datastore schema version for this object. This can be used
	// to facilitate schema migrations.
	//
	// The current schema is currentSchemaVersion.
	Schema string // index needed for batch conversions

	// Prefix is this log stream's prefix value. Log streams with the same prefix
	// are logically grouped.
	//
	// This value should not be changed once populated, as it will invalidate the
	// ID.
	Prefix string // index needed for Query RPC
	// Name is the unique name of this log stream within the Prefix scope.
	//
	// This value should not be changed once populated, as it will invalidate the
	// ID.
	Name string `gae:",noindex"`

	// Created is the time when this stream was created.
	Created time.Time `gae:",noindex"`
	// ExpireAt is time after which the datastore entry for the stream will be deleted.
	ExpireAt time.Time `gae:",noindex"`

	// Purged, if true, indicates that this log stream has been marked as purged.
	// Non-administrative queries and requests for this stream will operate as
	// if this entry doesn't exist.
	Purged bool `gae:",noindex"`
	// PurgedTime is the time when this stream was purged.
	PurgedTime time.Time `gae:",noindex"`

	// ProtoVersion is the version string of the protobuf, as reported by the
	// Collector (and ultimately self-identified by the Butler).
	ProtoVersion string `gae:",noindex"`
	// Descriptor is the binary protobuf data LogStreamDescriptor.
	Descriptor []byte `gae:",noindex"`

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
	// Drop old _C and _Tags fields to save memory.
	//   * _C is is derived entirely from Prefix and Name
	//   * _Tags is derived entirely from Descriptor
	//   * Tags is derived entirely from Descriptor (and briefly appeared in
	//     schema version 2)
	delete(pmap, "_C")
	delete(pmap, "_Tags")
	delete(pmap, "Tags")

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

	return ds.GetPLS(s).Save(withMeta)
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
	if s.Created.IsZero() {
		return errors.New("created time is not set")
	}

	// Ensure that our Descriptor can be unmarshalled.
	if _, err := s.DescriptorProto(); err != nil {
		return fmt.Errorf("could not unmarshal descriptor: %v", err)
	}
	return nil
}

// LoadDescriptor loads the fields in the log stream descriptor into this
// LogStream entry. These fields are:
//   - Prefix
//   - Name
//   - Descriptor
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
	s.Descriptor = pb

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

// LogStreamQuery is a function returning `true` if the provided LogStream
// matches.
type LogStreamQuery struct {
	Prefix types.StreamName // the prefix being queried

	q             *ds.Query
	includePurged bool
	checks        []func(*LogStream) bool
	descChecks    []func(*logpb.LogStreamDescriptor) bool
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
		return nil, errors.Fmt("prefix invalid: %w", err)
	}

	if name == "" {
		name = "**"
	}
	if err := types.StreamName(strings.Replace(string(name), "*", "a", -1)).Validate(); err != nil {
		return nil, errors.Fmt("name invalid: %w", err)
	}

	ret := &LogStreamQuery{
		Prefix: prefix,
		q:      ds.NewQuery("LogStream").Eq("Prefix", string(prefix)),
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
		return nil, errors.Fmt("compiling name regex: %w", err)
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
	lsp.descChecks = append(lsp.descChecks, func(desc *logpb.LogStreamDescriptor) bool {
		return desc.ContentType == ctype
	})
}

// OnlyStreamType constrains the LogStreamQuery to only return LogStreams of
// the given stream type.
func (lsp *LogStreamQuery) OnlyStreamType(stype logpb.StreamType) error {
	if _, ok := logpb.StreamType_name[int32(stype)]; !ok {
		return errors.New("unknown StreamType")
	}
	lsp.descChecks = append(lsp.descChecks, func(desc *logpb.LogStreamDescriptor) bool {
		return desc.StreamType == stype
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

// MustHaveTags constrains LogStreams returned to have all of the given tags.
func (lsp *LogStreamQuery) MustHaveTags(tags map[string]string) {
	lsp.descChecks = append(lsp.descChecks, func(desc *logpb.LogStreamDescriptor) bool {
		for k, v := range tags {
			actual, ok := desc.Tags[k]
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
	if len(lsp.descChecks) > 0 {
		desc, err := ls.DescriptorProto()
		if err != nil {
			return false
		}

		for _, checkFn := range lsp.descChecks {
			if !checkFn(desc) {
				return false
			}
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
