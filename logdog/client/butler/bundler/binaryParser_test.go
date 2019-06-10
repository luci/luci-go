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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/logdog/api/logpb"
)

func TestBinaryParser(t *testing.T) {
	Convey(`A binaryParser with a threshold of 2.`, t, func() {
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

		Convey(`Loaded with data below the threshold`, func() {
			p.Append(data(s.now, 1))
			Convey(`Returns nil when reading data smaller than the threshold.`, func() {
				le, err := p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, ShouldBeNil)
			})

			Convey(`Returns a LogEntry when truncating.`, func() {
				c.allowSplit = true
				le, err := p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, shouldMatchLogEntry, s.le(0, logpb.Binary{
					Data: []byte{1},
				}))

				le, err = p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, ShouldBeNil)
			})
		})

		Convey(`Loaded with 10 bytes of data`, func() {
			p.Append(data(s.now, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9))

			Convey(`Should yield all data with a limit of 32.`, func() {
				le, err := p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, shouldMatchLogEntry, s.le(0, logpb.Binary{
					Data: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9},
				}))

				le, err = p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, ShouldBeNil)
			})

			Convey(`Should yield [0..5], [6..9] with a limit of 6.`, func() {
				c.limit = 6
				le, err := p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, shouldMatchLogEntry, s.le(0, logpb.Binary{
					Data: []byte{0, 1, 2, 3, 4, 5},
				}))

				le, err = p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, shouldMatchLogEntry, s.le(6, logpb.Binary{
					Data: []byte{6, 7, 8, 9},
				}))

				le, err = p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, ShouldBeNil)
			})
		})

		Convey(`Loaded with 8 bytes of data from different times.`, func() {
			for i := 0; i < 8; i++ {
				p.Append(data(s.now.Add(time.Duration(i)*time.Second), byte(i)))
			}

			Convey(`Ignores the time boundary and returns all 8 bytes.`, func() {
				le, err := p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, shouldMatchLogEntry, s.le(0, logpb.Binary{
					Data: []byte{0, 1, 2, 3, 4, 5, 6, 7},
				}))

				le, err = p.nextEntry(c)
				So(err, ShouldBeNil)
				So(le, ShouldBeNil)
			})
		})
	})
}
