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

package lib

import (
	"os"

	"go.etcd.io/bbolt"
	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
)

type bboltCache struct {
	db *bbolt.DB

	// If size of db file exceeds this limit when closing, this cache removes db file.
	sizeLimit int64
}

var bucketName = []byte("v1")

func newbboltCache(name string, sizeLimit int64) (*bboltCache, error) {
	db, err := bbolt.Open(name, 0600, nil)
	if err != nil {
		return nil, err
	}

	// This is necessary to reduce disk access during db operations.
	db.NoSync = true
	db.NoFreelistSync = true

	if err := db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists(bucketName)
		return err
	}); err != nil {
		db.Close()
		return nil, err
	}

	return &bboltCache{
		db:        db,
		sizeLimit: sizeLimit,
	}, nil
}

func (c *bboltCache) close() error {
	path := c.db.Path()
	if err := c.db.Close(); err != nil {
		return errors.Annotate(err, "failed to close db").Err()
	}

	fi, err := os.Stat(path)
	if err != nil {
		return errors.Annotate(err, "failed to get stat").Err()
	}

	if fi.Size() <= c.sizeLimit {
		return nil
	}

	if err := os.Remove(path); err != nil {
		return errors.Annotate(err, "failed to remove file").Err()
	}

	return nil
}

func (c *bboltCache) put(key, value []byte) error {
	return c.db.Batch(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketName).Put(key, value)
	})
}

// getMulti calls |fn| in parallel for cached entries.
func (c *bboltCache) getMulti(keys [][]byte, fn func(key, value []byte) error) error {
	return c.db.View(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketName)

		var eg errgroup.Group

		for _, key := range keys {
			value := bucket.Get(key)
			if value == nil {
				continue
			}
			key := key
			eg.Go(func() error {
				return fn(key, value)
			})
		}

		return eg.Wait()
	})
}
