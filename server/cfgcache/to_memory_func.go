// Copyright 2026 The LUCI Authors.
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

package cfgcache

import (
	"context"
	"reflect"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config"
)

type (
	// ToMemoryFunc is the type of function used for [ServicePipeline].ToMemory and
	// [ProjectPipeline].ToMemory.
	ToMemoryFunc[StoreMsg proto.Message, InMemory any] func(ctx context.Context, cs config.Set, msg StoreMsg) (InMemory, error)

	// genericToMemoryFunc is a generic transmutation of [ToMemoryFunc].
	//
	// NOTE: Unlike ToMemoryFunc, this does NOT return the value - the reason for
	// this is the different in-memory representation of ServicePipeline vs
	// ProjectPipeline (atomic.Pointer and sync.Map, respectively). These
	// pipelines will supply the actual storage callback when generating this
	// generic function representation, and ingestConfig will just use this as a
	// side-effecting callback.
	genericToMemoryFunc func(context.Context, config.Set, proto.Message) error

	// NoMemoryRefresh is a sentinel type for the InMemory type parameter of
	// [ServicePipeline] and [ProjectPipeline] indicating that the configuration
	// is only ingested into storage and not refreshed into memory.
	//
	// When InMemory is NoMemoryRefresh, [ServicePipeline.Register] and
	// [ProjectPipeline.Register] return nil.
	NoMemoryRefresh struct{}
)

func (s ToMemoryFunc[StoreMsg, InMemory]) asGeneric(store func(config.Set, *InMemory)) (genericToMemoryFunc, error) {
	inMemType := reflect.TypeFor[InMemory]()
	if inMemType == reflect.TypeFor[NoMemoryRefresh]() {
		if s != nil {
			return nil, errors.New("ToMemory must not be set when InMemory is NoMemoryRefresh")
		}
		return nil, nil
	}

	if s == nil {
		if reflect.TypeFor[StoreMsg]() != inMemType {
			return nil, errors.New("ToMemory must be set when StoreMsg and InMemory types differ")
		}
		s = func(_ context.Context, _ config.Set, m StoreMsg) (InMemory, error) {
			return any(m).(InMemory), nil
		}
	}

	return func(ctx context.Context, cs config.Set, m proto.Message) error {
		if m == nil {
			store(cs, nil)
			return nil
		}
		inMem, err := s(ctx, cs, m.(StoreMsg))
		if err != nil {
			return err
		}
		store(cs, &inMem)
		return nil
	}, nil
}
