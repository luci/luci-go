// Copyright 2018 The LUCI Authors.
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

package mapper

import (
	"encoding/base64"
	"encoding/json"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
)

// Params are submitted with a job and passed to the mapper as is uninterpreted.
//
// Values must be JSON-serializable. They'll be deserialized using default JSON
// deserializer, type information will be lost (e.g. typed int parameter will
// be deserialized as interface{}).
type Params map[string]interface{}

// ToProperty is part of datastore.PropertyConverter interface.
func (p *Params) ToProperty() (prop datastore.Property, err error) {
	var blob []byte
	if p == nil || len(*p) == 0 {
		blob = []byte(`{}`)
	} else if blob, err = json.Marshal(p); err != nil {
		return
	}
	err = prop.SetValue(blob, datastore.NoIndex)
	return
}

// FromProperty is part of datastore.PropertyConverter interface.
func (p *Params) FromProperty(prop datastore.Property) error {
	*p = nil
	switch prop.Type() {
	case datastore.PTNull:
		return nil
	case datastore.PTBytes, datastore.PTString:
		blob, err := prop.Project(datastore.PTBytes)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(blob.([]byte), p); err != nil {
			return err
		}
		if len(*p) == 0 { // don't bother storing {}, it just messes with tests
			*p = nil
		}
		return nil
	default:
		return errors.Reason("don't know how to load %s into Params", prop.Type()).Err()
	}
}

var _ datastore.PropertyConverter = (*Params)(nil)

// SetProto stores a proto message as serialized string under the given key.
//
// Returns an error if the proto can't be serialized.
func (p *Params) SetProto(key string, msg proto.Message) error {
	blob, err := proto.Marshal(msg)
	if err != nil {
		return errors.Annotate(err, "failed to serialize proto under key %q", key).Err()
	}
	if *p == nil {
		*p = make(Params, 1)
	}
	(*p)[key] = base64.RawStdEncoding.EncodeToString(blob)
	return nil
}

// GetProto reads a proto message previously stored with SetProto.
//
// Returns an error if there's no such key or the proto can't be deserialized.
func (p *Params) GetProto(key string, msg proto.Message) error {
	val, ok := (*p)[key]
	if !ok {
		return errors.Reason("no property %q in props", key).Err()
	}
	b64, ok := val.(string)
	if !ok {
		return errors.Reason("property under key %q is not a string", key).Err()
	}
	blob, err := base64.RawStdEncoding.DecodeString(b64)
	if err != nil {
		return errors.Annotate(err, "failed to base64-decode proto under key %q", key).Err()
	}
	if err := proto.Unmarshal(blob, msg); err != nil {
		return errors.Annotate(err, "failed to deserialized proto under key %q", key).Err()
	}
	return nil
}
