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

	"go.opencensus.io/trace"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSamplerParsing(t *testing.T) {
	t.Parallel()

	Convey("Percent", t, func() {
		s, err := Sampler("0.1%")
		So(err, ShouldBeNil)
		So(s, ShouldNotBeNil)

		_, err = Sampler("abc%")
		So(err, ShouldErrLike, `not a float percent "abc%"`)
	})

	Convey("QPS", t, func() {
		s, err := Sampler("0.1qps")
		So(err, ShouldBeNil)
		So(s, ShouldNotBeNil)

		_, err = Sampler("abc%")
		So(err, ShouldErrLike, `not a float percent "abc%"`)
	})

	Convey("Unrecognized", t, func() {
		_, err := Sampler("huh")
		So(err, ShouldErrLike, "unrecognized sampling spec string")
	})
}

func TestQPSSampler(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		now := atomic.Value{}
		now.Store(time.Now()) // the absolute value doesn't matter
		tick := func(dt time.Duration) { now.Store(now.Load().(time.Time).Add(dt)) }

		sampler := qpsSampler{
			period: time.Second, // sample one request per second
			now:    func() time.Time { return now.Load().(time.Time) },
		}

		sampled := 0
		for i := 0; i < 10000; i++ {
			// Note: TraceID is not used in the current implementation, but we supply
			// it nonetheless to make the test also work with other implementations.
			params := trace.SamplingParameters{}
			if _, err := rand.Read(params.TraceID[:]); err != nil {
				panic(err)
			}
			if sampler.Sampler(params).Sample {
				sampled++
			}
			tick(10 * time.Millisecond) // 100 QPS
		}
		So(sampled, ShouldEqual, 10000/100)
	})
}
