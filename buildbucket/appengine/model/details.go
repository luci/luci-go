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

	"github.com/golang/protobuf/proto"
	structpb "github.com/golang/protobuf/ptypes/struct"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/buildbucket/proto"
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

// Ensure DSStruct implements datastore.PropertyConverter.
var _ datastore.PropertyConverter = &DSStruct{}

// DSStruct is a wrapper around structpb.Struct.
// Implements datastore.PropertyConverter,
// allowing reads from and writes to the datastore.
type DSStruct struct {
	structpb.Struct
}

// FromProperty deserializes structpb.Struct protos from the datastore.
// Implements datastore.PropertyConverter.
func (s *DSStruct) FromProperty(p datastore.Property) error {
	err := proto.Unmarshal(p.Value().([]byte), &s.Struct)
	if err != nil {
		return err
	}
	defaultStructValues(&s.Struct)
	return nil
}

// ToProperty serializes structpb.Struct protos to datastore format.
// Implements datastore.PropertyConverter.
func (s *DSStruct) ToProperty() (datastore.Property, error) {
	p := datastore.Property{}
	b, err := proto.Marshal(&s.Struct)
	if err != nil {
		return p, errors.Annotate(err, "failed to marshal proto").Err()
	}
	// noindex is not respected in tags.
	return p, p.SetValue(b, datastore.NoIndex)
}

// DSBuildInfra is a wrapper around pb.BuildInfra.
// Although pb.BuildInfra already implements datastore.PropertyConverter,
// override FromProto to apply defaultStructValues.
type DSBuildInfra struct {
	pb.BuildInfra
}

// FromProperty deserializes pb.BuildInfra protos from the datastore.
// Implements datastore.PropertyConverter.
func (b *DSBuildInfra) FromProperty(p datastore.Property) error {
	err := b.BuildInfra.FromProperty(p)
	if err != nil {
		return err
	}
	if b.GetBuildbucket() != nil {
		defaultStructValues(b.Buildbucket.RequestedProperties)
	}
	return nil
}

// BuildInfra is a representation of a build proto's infra field
// in the datastore.
type BuildInfra struct {
	_kind string `gae:"$kind,BuildInfra"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the pb.BuildInfra proto representation of the infra field.
	Proto DSBuildInfra `gae:"infra,noindex"`
}

// BuildInputProperties is a representation of a build proto's input field's
// properties field in the datastore.
type BuildInputProperties struct {
	_kind string `gae:"$kind,BuildInputProperties"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the struct.Struct representation of the properties field.
	Proto DSStruct `gae:"properties,noindex"`
}

// BuildOutputProperties is a representation of a build proto's output field's
// properties field in the datastore.
type BuildOutputProperties struct {
	_kind string `gae:"$kind,BuildOutputProperties"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// Proto is the struct.Struct representation of the properties field.
	Proto DSStruct `gae:"properties,noindex"`
}

// BuildSteps is a representation of a build proto's steps field
// in the datastore.
type BuildSteps struct {
	_kind string `gae:"$kind,BuildSteps"`
	// ID is always 1 because only one such entity exists.
	ID int `gae:"$id"`
	// Build is the key for the build this entity belongs to.
	Build *datastore.Key `gae:"$parent"`
	// IsZipped indicates whether or not Bytes must be zlib decompressed.
	IsZipped bool `gae:"step_container_bytes_zipped,noindex"`
	// Bytes is the pb.Build proto representation of the build proto where only steps is set.
	// IsZipped determines whether this value is compressed or not.
	Bytes []byte `gae:"steps,noindex"`
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
	}
	p := &pb.Build{}
	if err := proto.Unmarshal(b, p); err != nil {
		return nil, errors.Annotate(err, "error unmarshalling %q", datastore.KeyForObj(ctx, s)).Err()
	}
	return p.Steps, nil
}
