// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
)

// LogStream is the primary datastore model containing information and state of
// an individual log stream.
//
// This structure contains the standard queryable fields, and is the source of
// truth for log stream state. Writes to LogStream should be done via Put, which
// will ensure that the LogStream's related query objects are kept in sync.
type LogStream struct {
	QueryBase

	// hashID is the cached generated ID from the stream's Prefix/Name fields. If
	// this is populated, ID metadata will be retrieved from this field instead of
	// generated.
	hashID string

	// Secret is the Butler secret value for this stream.
	//
	// This value may only be returned to LogDog services; it is not user-visible.
	Secret []byte `gae:",noindex"`

	// Created is the time when this stream was created.
	Created time.Time
	// Updated is the Coordinator's record of when this log stream was last
	// updated.
	Updated time.Time

	// Timestamp is the Descriptor's recorded client-side timestamp.
	Timestamp time.Time

	// Descriptor is the binary protobuf data LogStreamDescriptor.
	Descriptor []byte `gae:",noindex"`

	// TerminalIndex is the log stream index of the last log entry in the stream.
	// If the value is -1, the log is still streaming.
	TerminalIndex int64 `gae:",noindex"`

	//ArchiveIndexURL is the Google Storage URL where the log stream's index is
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
	if err := types.StreamPath(value).Validate(); err != nil {
		// If it's not a path, see if it's a SHA256 sum.
		hash, hashErr := normalizeHash(value)
		if hashErr != nil {
			return nil, fmt.Errorf("invalid path (%s) and hash (%s)", err, hashErr)
		}

		// Load this LogStream with its SHA256 hash directly. This stream will not
		// have its Prefix/Name fields populated until it's loaded from datastore.
		return LogStreamFromID(hash), nil
	}

	// Load the prefix/name fields into the log stream.
	prefix, name := types.StreamPath(value).Split()
	return &LogStream{
		QueryBase: QueryBase{
			Prefix: string(prefix),
			Name:   string(name),
		},
	}, nil
}

// LogStreamFromID returns an empty LogStream instance with a known hash ID.
func LogStreamFromID(hashID string) *LogStream {
	return &LogStream{
		hashID: hashID,
	}
}

// Load implements ds.PropertyLoadSaver.
func (s *LogStream) Load(pmap ds.PropertyMap) error {
	if err := s.QueryBase.PLSLoadFrom(pmap); err != nil {
		return err
	}
	return ds.GetPLS(s).Load(pmap)
}

// Save implements ds.PropertyLoadSaver.
func (s *LogStream) Save(withMeta bool) (ds.PropertyMap, error) {
	pmap, err := ds.GetPLS(s).Save(withMeta)
	if err != nil {
		return nil, err
	}
	if err := s.QueryBase.PLSSaveTo(pmap, withMeta); err != nil {
		return nil, err
	}
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
	case protocol.LogStreamDescriptor_TEXT, protocol.LogStreamDescriptor_BINARY,
		protocol.LogStreamDescriptor_DATAGRAM:
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

// Archived returns true if this stream has been archived. A stream is archived
// if it has any of its archival properties set.
func (s *LogStream) Archived() bool {
	return !s.ArchiveMatches("", "", "")
}

// Terminated returns true if this stream has been terminated.
func (s *LogStream) Terminated() bool {
	return s.TerminalIndex >= 0
}

// ArchiveMatches tests if hte supplied Stream, Index, and Data archival URLs
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
func (s *LogStream) LoadDescriptor(desc *protocol.LogStreamDescriptor) error {
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
		s.Timestamp = normalizeTime(ts.Time().UTC())
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
func (s *LogStream) DescriptorProto() (*protocol.LogStreamDescriptor, error) {
	desc := protocol.LogStreamDescriptor{}
	if err := proto.Unmarshal(s.Descriptor, &desc); err != nil {
		return nil, err
	}
	return &desc, nil
}

// Put adds this LogStream instance to the datastore. It also generates and
// updates the derived QueryBases for this LogStream instance. It should
// be executed as part of a cross-group transaction so that the updates are
// transactionally applied.
func (s *LogStream) Put(di ds.Interface) error {
	if err := s.Validate(); err != nil {
		return err
	}

	// Update our QueryBase to include derived fields.
	s.QueryBase.Archived = s.Archived()
	s.QueryBase.Terminated = s.Terminated()

	// Add timestamp-indexed LogStream based on Created timestamp.
	tiqf := &TimestampIndexedQueryFields{
		QueryBase: s.QueryBase,
		LogStream: di.KeyForObj(s),
		Timestamp: s.Created,
	}

	if err := di.PutMulti([]interface{}{s, tiqf}); err != nil {
		return err
	}
	return nil
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
