// Copyright 2024 The LUCI Authors.
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

package dsutils

import (
	"io"
	"reflect"

	"github.com/apache/beam/sdks/v2/go/pkg/beam"
	"github.com/apache/beam/sdks/v2/go/pkg/beam/core/graph/coder"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
)

var (
	datastoreKeyPointerType = reflect.TypeOf((*datastore.Key)(nil))
	datastoreKeyType        = datastoreKeyPointerType.Elem()
	datastoreKeyStorageType = reflect.TypeOf((*datastoreKeyStorage)(nil)).Elem()
)

func init() {
	// A coder is required when *datastore.Key is stored in a persisted storage
	// other than a PCollection.
	beam.RegisterCoder(datastoreKeyPointerType, encodeDatastoreKey, decodeDatastoreKey)
	beam.RegisterSchemaProvider(datastoreKeyType, &datastoreKeyProvider{})
}

func encodeDatastoreKey(key *datastore.Key) []byte {
	if key == nil {
		return nil
	}

	return []byte(key.Encode())
}

func decodeDatastoreKey(bytes []byte) (*datastore.Key, error) {
	if len(bytes) == 0 {
		return nil, nil
	}

	return datastore.NewKeyEncoded(string(bytes))
}

type datastoreKeyStorage struct {
	Key []byte
}

type datastoreKeyProvider struct{}

// FromLogicalType implements SchemaProvider.
func (p *datastoreKeyProvider) FromLogicalType(rt reflect.Type) (reflect.Type, error) {
	if rt != datastoreKeyType {
		return nil, errors.Reason("unable to provide schema.LogicalType for type %v, want %v", rt, datastoreKeyType).Err()
	}
	return datastoreKeyStorageType, nil
}

// BuildEncoder implements SchemaProvider.
func (p *datastoreKeyProvider) BuildEncoder(rt reflect.Type) (func(any, io.Writer) error, error) {
	if _, err := p.FromLogicalType(rt); err != nil {
		return nil, err
	}
	enc, err := coder.RowEncoderForStruct(datastoreKeyStorageType)
	if err != nil {
		return nil, err
	}
	return func(iface any, w io.Writer) error {
		v := iface.(datastore.Key)
		return enc(datastoreKeyStorage{
			Key: []byte(v.Encode()),
		}, w)
	}, nil
}

// BuildDecoder implements SchemaProvider.
func (p *datastoreKeyProvider) BuildDecoder(rt reflect.Type) (func(io.Reader) (any, error), error) {
	if _, err := p.FromLogicalType(rt); err != nil {
		return nil, err
	}
	dec, err := coder.RowDecoderForStruct(datastoreKeyStorageType)
	if err != nil {
		return nil, err
	}
	return func(r io.Reader) (any, error) {
		s, err := dec(r)
		if err != nil {
			return nil, err
		}
		tn := s.(datastoreKeyStorage)
		if len(tn.Key) == 0 {
			return nil, nil
		}

		key, err := datastore.NewKeyEncoded(string(tn.Key))
		return *key, err
	}, nil
}
