// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"math/rand"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/sdk/trace"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSamplerParsing(t *testing.T) {
	t.Parallel()

	ftt.Run("Percent", t, func(t *ftt.Test) {
		s, err := BaseSampler("0.1%")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.NotBeNil)

		_, err = BaseSampler("abc%")
		assert.Loosely(t, err, should.ErrLike(`not a float percent "abc%"`))
	})

	ftt.Run("QPS", t, func(t *ftt.Test) {
		s, err := BaseSampler("0.1qps")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, s, should.NotBeNil)

		_, err = BaseSampler("abcqps")
		assert.Loosely(t, err, should.ErrLike(`not a float QPS "abcqps"`))
	})

	ftt.Run("Unrecognized", t, func(t *ftt.Test) {
		_, err := BaseSampler("huh")
		assert.Loosely(t, err, should.ErrLike("unrecognized sampling spec string"))
	})
}

func TestQPSSampler(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		now := atomic.Value{}
		now.Store(time.Now()) // the absolute value doesn't matter
		tick := func(dt time.Duration) { now.Store(now.Load().(time.Time).Add(dt)) }

		sampler := qpsSampler{
			period: time.Second, // sample one request per second
			now:    func() time.Time { return now.Load().(time.Time) },
			rnd:    rand.New(rand.NewSource(0)),
		}

		sampled := 0
		for i := 0; i < 10000; i++ {
			// Note: TraceID is not used in the current implementation, but we supply
			// it nonetheless to make the test also work with other implementations.
			params := trace.SamplingParameters{}
			if _, err := rand.Read(params.TraceID[:]); err != nil {
				panic(err)
			}
			if sampler.ShouldSample(params).Decision == trace.RecordAndSample {
				sampled++
			}
			tick(10 * time.Millisecond) // 100 QPS
		}
		assert.Loosely(t, sampled, should.Equal(10000/100+1)) // '+1' is due to randomization
	})
}
