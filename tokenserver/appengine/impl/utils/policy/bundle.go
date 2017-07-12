// Copyright 2017 The LUCI Authors.
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

package policy

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"sort"

	"github.com/golang/protobuf/proto"
)

// ConfigBundle is a bunch of related parsed text proto files.
//
// For example, it may be a main top-level config and a bunch of include files
// it references.
//
// Keys are paths, values are corresponding proto messages. Users are supposed
// to know themselves what concrete proto types correspond to what paths.
type ConfigBundle map[string]proto.Message

// blobWithType is underlying type of a slice we gob-serialize in serialize().
type blobWithType struct {
	Path string // the path the config was fetched from
	Kind string // proto message kind, to know how to deserialize
	Blob []byte // proto message, binary serialization
}

// serializeBundle deterministically converts ConfigBundle into a byte blob.
//
// The byte blob references proto message names to know how to deserialize them
// later.
func serializeBundle(b ConfigBundle) ([]byte, error) {
	keys := make([]string, 0, len(b))
	for k := range b {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]blobWithType, 0, len(b))
	for _, k := range keys {
		v := b[k]
		blob, err := proto.Marshal(v)
		if err != nil {
			return nil, err
		}
		items = append(items, blobWithType{
			Path: k,
			Kind: proto.MessageName(v),
			Blob: blob,
		})
	}

	out := bytes.Buffer{}
	if err := gob.NewEncoder(&out).Encode(items); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// deserialize parses the serialized ConfigBundle.
//
// It skips configs with proto types no longer registered in the proto lib
// registry. It returns them unparsed in 'unknown' slice.
//
// Returns an error if some known proto message can't be deserialized.
func deserializeBundle(blob []byte) (b ConfigBundle, unknown []blobWithType, err error) {
	items := []blobWithType{}
	if err := gob.NewDecoder(bytes.NewReader(blob)).Decode(&items); err != nil {
		return nil, nil, err
	}

	b = make(ConfigBundle, len(items))
	for _, item := range items {
		t := proto.MessageType(item.Kind) // this is *SomeProto type
		if t == nil {
			unknown = append(unknown, item)
			continue
		}
		msg := reflect.New(t.Elem()).Interface().(proto.Message)
		if err := proto.Unmarshal(item.Blob, msg); err != nil {
			return nil, nil, err
		}
		b[item.Path] = msg
	}

	return b, unknown, nil
}
