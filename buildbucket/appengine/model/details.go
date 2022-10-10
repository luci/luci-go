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

package model

import (
	"bytes"
	"compress/zlib"
	"context"
	"io/ioutil"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
)

// defaultStructValues defaults nil or empty values inside the given
// structpb.Struct. Needed because structpb.Value cannot be marshaled to JSON
// unless there is a kind set.
func defaultStructValues(s *structpb.Struct) {
	for k, v := range s.GetFields() {
		switch {
		case v == nil:
			s.Fields[k] = &structpb.Value{
				Kind: &structpb.Value_NullValue{},
			}
		case v.Kind == nil:
			v.Kind = &structpb.Value_NullValue{}
		case v.GetStructValue() != nil:
			defaultStructValues(v.GetStructValue())
		}
	}
}

// BuildInfra is a representation of a build proto's infra field
// in the datastore.
type BuildInfra struct {
	_kind string `gae:"$kind,BuildInfra"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the pb.BuildInfra proto representation of the infra field.
	Proto *pb.BuildInfra `gae:"infra,legacy"`
}

var _ datastore.PropertyLoadSaver = (*BuildInfra)(nil)

// Load implements datastore.PropertyLoadSaver in order to apply
// defaultStructValues to bi.Proto.
func (bi *BuildInfra) Load(pm datastore.PropertyMap) error {
	if err := datastore.GetPLS(bi).Load(pm); err != nil {
		return err
	}
	if bi.Proto.GetBuildbucket() != nil {
		defaultStructValues(bi.Proto.Buildbucket.RequestedProperties)
	}
	return nil
}

// Save implements datastore.PropertyLoadSaver
func (bi *BuildInfra) Save(withMeta bool) (datastore.PropertyMap, error) {
	return datastore.GetPLS(bi).Save(withMeta)
}

// BuildInputProperties is a representation of a build proto's input field's
// properties field in the datastore.
type BuildInputProperties struct {
	_kind string `gae:"$kind,BuildInputProperties"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the structpb.Struct representation of the properties field.
	Proto *structpb.Struct `gae:"properties,legacy"`
}

// BuildOutputProperties is a representation of a build proto's output field's
// properties field in the datastore.
type BuildOutputProperties struct {
	_ datastore.PropertyMap `gae:"-,extra"`
	_kind string `gae:"$kind,BuildOutputProperties"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the structpb.Struct representation of the properties field.
	Proto *structpb.Struct `gae:"properties,legacy"`
}

// BuildStepsMaxBytes is the maximum length of BuildSteps.Bytes. If Bytes
// exceeds this maximum, this package will try to compress it, setting IsZipped
// accordingly, but if this length is still exceeded it's an error to write
// such entities to the datastore. Use FromProto to ensure this maximum is
// respected.
const BuildStepsMaxBytes = 1e6

// BuildSteps is a representation of a build proto's steps field
// in the datastore.
type BuildSteps struct {
	_kind string `gae:"$kind,BuildSteps"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id,1"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// IsZipped indicates whether or not Bytes is zlib compressed.
	// Use ToProto to ensure this compression is respected.
	IsZipped bool `gae:"step_container_bytes_zipped,noindex"`
	// Bytes is the pb.Build proto representation of the build proto where only steps is set.
	// IsZipped determines whether this value is compressed or not.
	Bytes []byte `gae:"steps,noindex"`
}

// CancelIncomplete marks any incomplete steps as cancelled, returning whether
// at least one step was cancelled. The caller is responsible for writing the
// entity to the datastore if any steps were cancelled. This entity will not be
// mutated if an error occurs.
func (s *BuildSteps) CancelIncomplete(ctx context.Context, now *timestamppb.Timestamp) (bool, error) {
	stp, err := s.ToProto(ctx)
	if err != nil {
		return false, err
	}
	changed := false
	for _, s := range stp {
		if !protoutil.IsEnded(s.Status) {
			s.EndTime = now
			s.Status = pb.Status_CANCELED
			changed = true
		}
	}
	if changed {
		if err := s.FromProto(stp); err != nil {
			return false, err
		}
	}
	return changed, nil
}

// FromProto overwrites the current []*pb.Step representation of these steps.
// The caller is responsible for writing the entity to the datastore. This
// entity will not be mutated if an error occurs.
func (s *BuildSteps) FromProto(stp []*pb.Step) error {
	b, err := proto.Marshal(&pb.Build{
		Steps: stp,
	})
	if err != nil {
		return errors.Annotate(err, "failed to marshal").Err()
	}
	if len(b) <= BuildStepsMaxBytes {
		s.Bytes = b
		s.IsZipped = false
		return nil
	}
	buf := &bytes.Buffer{}
	w := zlib.NewWriter(buf)
	if _, err := w.Write(b); err != nil {
		return errors.Annotate(err, "error zipping").Err()
	}
	if err := w.Close(); err != nil {
		return errors.Annotate(err, "error closing writer").Err()
	}
	s.Bytes = buf.Bytes()
	s.IsZipped = true
	return nil
}

// ToProto returns the []*pb.Step representation of these steps.
func (s *BuildSteps) ToProto(ctx context.Context) ([]*pb.Step, error) {
	b := s.Bytes
	if s.IsZipped {
		r, err := zlib.NewReader(bytes.NewReader(s.Bytes))
		if err != nil {
			return nil, errors.Annotate(err, "error creating reader for %q", datastore.KeyForObj(ctx, s)).Err()
		}
		b, err = ioutil.ReadAll(r)
		if err != nil {
			return nil, errors.Annotate(err, "error reading %q", datastore.KeyForObj(ctx, s)).Err()
		}
		if err := r.Close(); err != nil {
			return nil, errors.Annotate(err, "error closing reader for %q", datastore.KeyForObj(ctx, s)).Err()
		}
	}
	p := &pb.Build{}
	if err := proto.Unmarshal(b, p); err != nil {
		return nil, errors.Annotate(err, "error unmarshalling %q", datastore.KeyForObj(ctx, s)).Err()
	}
	return p.Steps, nil
}
