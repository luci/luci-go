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

package datastore

import (
	"fmt"

	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

type checkFilter struct {
	RawInterface

	kc KeyContext
}

func (tcf *checkFilter) RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error {
	if f == nil {
		return fmt.Errorf("datastore: RunInTransaction function is nil")
	}
	return tcf.RawInterface.RunInTransaction(f, opts)
}

func (tcf *checkFilter) Run(fq *FinalizedQuery, cb RawRunCB) error {
	if fq == nil {
		return fmt.Errorf("datastore: Run query is nil")
	}
	if cb == nil {
		return fmt.Errorf("datastore: Run callback is nil")
	}
	return tcf.RawInterface.Run(fq, cb)
}

func (tcf *checkFilter) GetMulti(keys []*Key, meta MultiMetaGetter, cb GetMultiCB) error {
	if cb == nil {
		return fmt.Errorf("datastore: GetMulti callback is nil")
	}

	var dat DroppedArgTracker

	for i, k := range keys {
		var err error
		switch {
		case k.IsIncomplete():
			err = MakeErrInvalidKey("key [%s] is incomplete", k).Err()
		case !k.Valid(true, tcf.kc):
			err = MakeErrInvalidKey("key [%s] is not valid in context %s", k, tcf.kc).Err()
		}
		if err != nil {
			cb(i, nil, err)
			dat.MarkForRemoval(i, len(keys))
		}
	}

	keys, meta, dal := dat.DropKeysAndMeta(keys, meta)
	if len(keys) == 0 {
		return nil
	}

	return tcf.RawInterface.GetMulti(keys, meta, func(idx int, val PropertyMap, err error) {
		cb(dal.OriginalIndex(idx), val, err)
	})
}

func (tcf *checkFilter) PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error {
	if len(keys) != len(vals) {
		return fmt.Errorf("datastore: PutMulti with mismatched keys/vals lengths (%d/%d)", len(keys), len(vals))
	}
	if cb == nil {
		return fmt.Errorf("datastore: PutMulti callback is nil")
	}

	var dat DroppedArgTracker
	for i, k := range keys {
		if !k.PartialValid(tcf.kc) {
			cb(i, nil, MakeErrInvalidKey("key [%s] is not partially valid in context %s", k, tcf.kc).Err())
			dat.MarkForRemoval(i, len(keys))
			continue
		}
		if vals[i] == nil {
			cb(i, nil, errors.New("datastore: PutMulti got nil vals entry"))
			dat.MarkForRemoval(i, len(keys))
		}
	}
	keys, vals, dal := dat.DropKeysAndVals(keys, vals)

	if len(keys) == 0 {
		return nil
	}
	return tcf.RawInterface.PutMulti(keys, vals, func(idx int, key *Key, err error) {
		cb(dal.OriginalIndex(idx), key, err)
	})
}

func (tcf *checkFilter) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	if cb == nil {
		return fmt.Errorf("datastore: DeleteMulti callback is nil")
	}
	var dat DroppedArgTracker
	for i, k := range keys {
		var err error
		switch {
		case k.IsIncomplete():
			err = MakeErrInvalidKey("key [%s] is incomplete", k).Err()
		case !k.Valid(false, tcf.kc):
			err = MakeErrInvalidKey("key [%s] is not valid in context %s", k, tcf.kc).Err()
		}
		if err != nil {
			cb(i, err)
			dat.MarkForRemoval(i, len(keys))
		}
	}
	keys, dal := dat.DropKeys(keys)
	if len(keys) == 0 {
		return nil
	}
	return tcf.RawInterface.DeleteMulti(keys, func(idx int, err error) {
		cb(dal.OriginalIndex(idx), err)
	})
}

func applyCheckFilter(c context.Context, i RawInterface) RawInterface {
	return &checkFilter{
		RawInterface: i,
		kc:           GetKeyContext(c),
	}
}
