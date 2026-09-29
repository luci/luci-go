// Copyright 2016 The LUCI Authors.
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

package certconfig

import (
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	ds "go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/shards"
)

func TestCRL(t *testing.T) {
	ftt.Run("CRL storage works", t, func(t *ftt.Test) {
		caName := "CA"
		shardCount := 4
		cachingTime := 10 * time.Second

		ctx := gaetesting.TestingContext()
		ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)

		// Prepare a set of CRLs (with holes, to be more close to life)
		crl := &pkix.CertificateList{}
		for i := 1; i < 100; i++ {
			crl.TBSCertList.RevokedCertificates = append(crl.TBSCertList.RevokedCertificates, pkix.RevokedCertificate{
				SerialNumber: big.NewInt(int64(i * 3)),
			})
		}

		// Upload it.
		assert.Loosely(t, UpdateCRLSet(ctx, caName, shardCount, crl), should.BeNil)

		// Use it.
		checker := NewCRLChecker(caName, shardCount, cachingTime)
		for i := 1; i < 300; i++ {
			revoked, err := checker.IsRevokedSN(ctx, big.NewInt(int64(i)))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, revoked, should.Equal((i%3) == 0))
		}

		// Cert #1 is revoked now too. It will invalidate one cache shard.
		crl.TBSCertList.RevokedCertificates = append(crl.TBSCertList.RevokedCertificates, pkix.RevokedCertificate{
			SerialNumber: big.NewInt(1),
		})

		// Upload it.
		assert.Loosely(t, UpdateCRLSet(ctx, caName, shardCount, crl), should.BeNil)

		// Old cache is still used.
		revoked, err := checker.IsRevokedSN(ctx, big.NewInt(1))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeFalse)

		// Roll time to invalidate the cache.
		clk.Add(cachingTime * 2)

		// New shard version is fetched.
		revoked, err = checker.IsRevokedSN(ctx, big.NewInt(1))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeTrue)

		// Hit a code path for refetching of an unchanged shard. Pick a SN that
		// doesn't belong to shard where '1' is.
		shardIdx := func(sn int64) int {
			blob, err := utils.SerializeSN(big.NewInt(sn))
			assert.Loosely(t, err, should.BeNil)
			return shards.ShardIndex(blob, shardCount)
		}
		forbiddenIdx := shardIdx(1)
		sn := int64(2)
		for shardIdx(sn) == forbiddenIdx {
			sn++
		}

		// Hit this shard.
		revoked, err = checker.IsRevokedSN(ctx, big.NewInt(sn))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.Equal((sn%3) == 0))
	})

	ftt.Run("Rollback Protection works", t, func(t *ftt.Test) {
		caName := "CA"
		shardCount := 1

		ctx := gaetesting.TestingContext()

		// Older CRL
		oldCRL := &pkix.CertificateList{
			TBSCertList: pkix.TBSCertificateList{
				ThisUpdate: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
				RevokedCertificates: []pkix.RevokedCertificate{
					{SerialNumber: big.NewInt(1)},
				},
			},
		}

		// Newer CRL
		newCRL := &pkix.CertificateList{
			TBSCertList: pkix.TBSCertificateList{
				ThisUpdate: time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC),
				RevokedCertificates: []pkix.RevokedCertificate{
					{SerialNumber: big.NewInt(2)},
				},
			},
		}

		// 1. Write the newer CRL
		assert.Loosely(t, UpdateCRLSet(ctx, caName, shardCount, newCRL), should.BeNil)

		// 2. Write the older CRL (simulates delayed execution).
		// It should evaluate safely but skip overwriting the datastore.
		assert.Loosely(t, UpdateCRLSet(ctx, caName, shardCount, oldCRL), should.BeNil)

		// 3. Verify shards reflect the newer CRL
		checker := NewCRLChecker(caName, shardCount, 0)

		// Cert 1 (from old CRL) should NOT be revoked.
		revoked, err := checker.IsRevokedSN(ctx, big.NewInt(1))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeFalse)

		// Cert 2 (from new CRL) SHOULD be revoked.
		revoked, err = checker.IsRevokedSN(ctx, big.NewInt(2))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeTrue)
	})

	ftt.Run("CRL freshness warnings work", t, func(t *ftt.Test) {
		caName := "CA"
		shardCount := 1
		cachingTime := 10 * time.Second

		ctx := gaetesting.TestingContext()
		ctx, clk := testclock.UseTime(ctx, testclock.TestTimeUTC)
		ctx = memlogger.Use(ctx)
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		assert.Loosely(t, UpdateCRLSet(ctx, caName, shardCount, &pkix.CertificateList{}), should.BeNil)

		crlEntity := CRL{
			Parent:         ds.NewKey(ctx, "CA", caName, 0, nil),
			LastFetchTime:  clock.Now(ctx).UTC(),
			LastUpdateTime: clock.Now(ctx).UTC(),
			NextUpdateTime: clock.Now(ctx).Add(24 * time.Hour).UTC(),
		}
		assert.Loosely(t, ds.Put(ctx, &crlEntity), should.BeNil)

		checker := NewCRLChecker(caName, shardCount, cachingTime)

		// Fresh CRL logs no warnings.
		logs.Reset()
		revoked, err := checker.IsRevokedSN(ctx, big.NewInt(1))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeFalse)
		assert.Loosely(t, memlogger.ShouldNotHaveLog(logs, logging.Warning, ""), should.BeEmpty)

		// Expired CRL logs a warning without failing IsRevokedSN.
		crlEntity.NextUpdateTime = clock.Now(ctx).Add(-1 * time.Minute).UTC()
		crlEntity.LastFetchTime = clock.Now(ctx).UTC()
		assert.Loosely(t, ds.Put(ctx, &crlEntity), should.BeNil)
		clk.Add(cachingTime + time.Second)

		logs.Reset()
		revoked, err = checker.IsRevokedSN(ctx, big.NewInt(1))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeFalse)
		assert.Loosely(t, memlogger.ShouldHaveLog(logs, logging.Warning, "expired at"), should.BeEmpty)

		// Stale CRL logs a warning without failing IsRevokedSN.
		crlEntity.NextUpdateTime = clock.Now(ctx).Add(24 * time.Hour).UTC()
		crlEntity.LastFetchTime = clock.Now(ctx).Add(-3 * time.Hour).UTC()
		assert.Loosely(t, ds.Put(ctx, &crlEntity), should.BeNil)
		clk.Add(cachingTime + time.Second)

		logs.Reset()
		revoked, err = checker.IsRevokedSN(ctx, big.NewInt(1))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, revoked, should.BeFalse)
		assert.Loosely(t, memlogger.ShouldHaveLog(logs, logging.Warning, "is stale"), should.BeEmpty)
	})
}
