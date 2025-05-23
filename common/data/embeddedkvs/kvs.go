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

package embeddedkvs

import (
	"context"

	"github.com/dgraph-io/badger/v3"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
)

type KVS struct {
	db *badger.DB
}

// New instantiates KVS.
func New(ctx context.Context, path string) (*KVS, error) {
	opt := badger.DefaultOptions(path).WithLoggingLevel(badger.WARNING)
	db, err := badger.Open(opt)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open database: %s", path).Err()
	}

	return &KVS{
		db: db,
	}, nil
}

// Close closes KVS.
func (k *KVS) Close() error {
	if err := k.db.Close(); err != nil {
		return errors.Annotate(err, "failed to close db").Err()
	}
	return nil
}

// Set sets key/value to storage.
//
// This should be called in parallel for efficient storing.
func (k *KVS) Set(key string, value []byte) error {
	if err := k.db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), value)
	}); err != nil {
		return errors.Annotate(err, "failed to put %s", key).Err()
	}
	return nil
}

// GetMulti calls |fn| in parallel for cached entries.
func (k *KVS) GetMulti(ctx context.Context, keys []string, fn func(key string, value []byte) error) error {
	if err := k.db.View(func(txn *badger.Txn) error {
		eg, _ := errgroup.WithContext(ctx)
		for _, key := range keys {
			eg.Go(func() error {
				item, err := txn.Get([]byte(key))
				if err == badger.ErrKeyNotFound {
					return nil
				}
				if err != nil {
					return errors.Annotate(err, "failed to get %s", key).Err()
				}
				return item.Value(func(val []byte) error {
					return fn(key, val)
				})

			})
		}

		return eg.Wait()
	}); err != nil {
		return errors.Annotate(err, "failed to get").Err()
	}

	return nil
}

// SetMulti receives callback that takes function which is used to set a key/value pair.
func (k *KVS) SetMulti(fn func(set func(key string, value []byte) error) error) error {
	wb := k.db.NewWriteBatch()
	defer wb.Cancel()

	if err := fn(func(key string, value []byte) error {
		if err := wb.Set([]byte(key), value); err != nil {
			return errors.Annotate(err, "failed to set key: %s", key).Err()
		}
		return nil
	}); err != nil {
		return err
	}

	if err := wb.Flush(); err != nil {
		return errors.Annotate(err, "failed to call Flush").Err()
	}
	return nil
}

// ForEach executes a function for each key/value pair in KVS.
func (k *KVS) ForEach(fn func(key string, value []byte) error) error {
	err := k.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()

			if err := item.Value(func(val []byte) error {
				return fn(string(item.Key()), val)
			}); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return errors.Annotate(err, "failed to iterate").Err()
	}
	return nil
}
