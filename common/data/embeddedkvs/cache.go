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
	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
)

type KVS struct {
	db *bbolt.DB
}

var bucketName = []byte("b")

// New instantiates KVS.
func New(path string) (*KVS, error) {
	db, err := bbolt.Open(path, 0600, nil)
	if err != nil {
		return nil, errors.Annotate(err, "failed to open database: %s", path).Err()
	}

	// This is necessary to reduce disk access during db operations.
	// https://pkg.go.dev/go.etcd.io/bbolt/#DB
	db.NoSync = true
	db.NoFreelistSync = true

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		if err != nil {
			return errors.Annotate(err, "failed to create bucket: %s", bucketName).Err()
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, errors.Annotate(err, "failed to initilize database").Err()
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
	if err := k.db.Batch(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketName).Put([]byte(key), value)
	}); err != nil {
		return errors.Annotate(err, "failed to put %s", key).Err()
	}
	return nil
}

// GetMulti calls |fn| in parallel for cached entries.
func (k *KVS) GetMulti(keys []string, fn func(key string, value []byte) error) error {
	if err := k.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketName)

		var eg errgroup.Group
		for _, key := range keys {
			value := bucket.Get([]byte(key))
			if value == nil {
				continue
			}
			key := key
			eg.Go(func() error {
				return fn(string(key), value)
			})
		}

		return eg.Wait()
	}); err != nil {
		return errors.Annotate(err, "failed to get").Err()
	}

	return nil
}
