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

package bundler

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

func TestBinaryParser(t *testing.T) {
	ftt.Run(`A binaryParser with a threshold of 2.`, t, func(t *ftt.Test) {
		s := &parserTestStream{
			now:         time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
			prefixIndex: 1337,
		}
		p := &binaryParser{
			baseParser: s.base(),
			threshold:  2,
		}
		c := &constraints{
			limit: 32,
		}

		t.Run(`Loaded with data below the threshold`, func(t *ftt.Test) {
			p.Append(data(s.now, 1))
			t.Run(`Returns nil when reading data smaller than the threshold.`, func(t *ftt.Test) {
				le, err := p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, le, should.BeNil)
			})

			t.Run(`Returns a LogEntry when truncating.`, func(t *ftt.Test) {
				c.allowSplit = true
				le, err := p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				shouldMatchLogEntry(t, le, s.le(0, logpb.Binary{Data: []byte{1}}))

				le, err = p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, le, should.BeNil)
			})
		})

		t.Run(`Loaded with 10 bytes of data`, func(t *ftt.Test) {
			p.Append(data(s.now, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9))

			t.Run(`Should yield all data with a limit of 32.`, func(t *ftt.Test) {
				le, err := p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				shouldMatchLogEntry(t, le, s.le(0, logpb.Binary{
					Data: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
				}))

				le, err = p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, le, should.BeNil)
			})

			t.Run(`Should yield [0..5], [6..9] with a limit of 6.`, func(t *ftt.Test) {
				c.limit = 6
				le, err := p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				shouldMatchLogEntry(t, le, s.le(0, logpb.Binary{
					Data: []byte{0, 1, 2, 3, 4, 5},
				}))

				le, err = p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				shouldMatchLogEntry(t, le, s.le(6, logpb.Binary{
					Data: []byte{6, 7, 8, 9},
				}))

				le, err = p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, le, should.BeNil)
			})
		})

		t.Run(`Loaded with 8 bytes of data from different times.`, func(t *ftt.Test) {
			for i := 0; i < 8; i++ {
				p.Append(data(s.now.Add(time.Duration(i)*time.Second), byte(i)))
			}

			t.Run(`Ignores the time boundary and returns all 8 bytes.`, func(t *ftt.Test) {
				le, err := p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				shouldMatchLogEntry(t, le, s.le(0, logpb.Binary{
					Data: []byte{0, 1, 2, 3, 4, 5, 6, 7},
				}))

				le, err = p.nextEntry(c)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, le, should.BeNil)
			})
		})
	})
}
