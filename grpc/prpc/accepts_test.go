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

package prpc

import (
	"net/http"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAccept(t *testing.T) {
	t.Parallel()

	ftt.Run("parseAccept", t, func(t *ftt.Test) {
		test := func(value string, expectedErr any, expectedTypes ...acceptItem) {
			t.Run("mediaType="+value, func(t *ftt.Test) {
				actual, err := parseAccept(value)
				assert.Loosely(t, err, should.ErrLike(expectedErr))
				assert.Loosely(t, actual, should.Match(accept(expectedTypes)))
			})
		}
		test("", nil)
		qf1 := func(value string) acceptItem {
			return acceptItem{
				Value:         value,
				QualityFactor: 1.0,
			}
		}

		test("text/html", nil,
			qf1("text/html"),
		)

		test("text/html; level=1", nil,
			qf1("text/html; level=1"),
		)
		test("TEXT/HTML; LEVEL=1", nil,
			qf1("TEXT/HTML; LEVEL=1"),
		)

		test("text/html, application/json", nil,
			qf1("text/html"),
			qf1("application/json"),
		)

		test("text/html; level=1, application/json", nil,
			qf1("text/html; level=1"),
			qf1("application/json"),
		)

		test("text/html; level=1, application/json; foo=bar", nil,
			qf1("text/html; level=1"),
			qf1("application/json; foo=bar"),
		)

		test("text/html; level=1; q=0.5", nil,
			acceptItem{
				Value:         "text/html; level=1",
				QualityFactor: 0.5,
			},
		)
		test("text/html; level=1; q=0.5; a=1", nil,
			acceptItem{
				Value:         "text/html; level=1",
				QualityFactor: 0.5,
			},
		)
		test("TEXT/HTML; LEVEL=1; Q=0.5", nil,
			acceptItem{
				Value:         "TEXT/HTML; LEVEL=1",
				QualityFactor: 0.5,
			},
		)
		test("text/html; level=1; q=0.5, application/json", nil,
			acceptItem{
				Value:         "text/html; level=1",
				QualityFactor: 0.5,
			},
			qf1("application/json"),
		)
		test("text/html; level=1; q=0.5, application/json; a=b", nil,
			acceptItem{
				Value:         "text/html; level=1",
				QualityFactor: 0.5,
			},
			qf1("application/json; a=b"),
		)
		test("text/html; level=1; q=0.5, */*; q=0.1", nil,
			acceptItem{
				Value:         "text/html; level=1",
				QualityFactor: 0.5,
			},
			acceptItem{
				Value:         "*/*",
				QualityFactor: 0.1,
			},
		)
		test("text/html; level=1; q=0.5, */*; x=3;q=0.1; y=5", nil,
			acceptItem{
				Value:         "text/html; level=1",
				QualityFactor: 0.5,
			},
			acceptItem{
				Value:         "*/*; x=3",
				QualityFactor: 0.1,
			},
		)
		test("text/html;q=q", "q parameter: expected a floating-point number")
	})

	ftt.Run("qParamSplit", t, func(t *ftt.Test) {
		test := func(item, mediaType, qValue string) {
			t.Run(item, func(t *ftt.Test) {
				actualMediaType, actualQValue := qParamSplit(item)
				assert.Loosely(t, actualMediaType, should.Equal(mediaType))
				assert.Loosely(t, actualQValue, should.Equal(qValue))
			})
		}

		test("", "", "")
		test("a/b", "a/b", "")

		test("a/b;mtp1=1", "a/b;mtp1=1", "")
		test("a/b;q=1", "a/b", "1")
		test("a/b; q=1", "a/b", "1")
		test("a/b;mtp1=1;q=1", "a/b;mtp1=1", "1")
		test("a/b;mtp1=1;q=1;", "a/b;mtp1=1", "1")
		test("a/b;mtp1=1; q=1", "a/b;mtp1=1", "1")
		test("a/b;mtp1=1;q =1", "a/b;mtp1=1", "1")
		test("a/b;mtp1=1;q  =1", "a/b;mtp1=1", "1")
		test("a/b;mtp1=1;q= 1", "a/b;mtp1=1", "1")
		test("a/b;mtp1=1;q=1 ", "a/b;mtp1=1", "1")

		test("a/b;mtp1=1;Q=1", "a/b;mtp1=1", "1")

		test("a/b;mtp1=1;q=1.0", "a/b;mtp1=1", "1.0")
		test("a/b;mtp1=1;q=1.0;", "a/b;mtp1=1", "1.0")

		test("a/b;mtp1=1;  q  =  foo", "a/b;mtp1=1", "foo")

		test("a/b;c=q", "a/b;c=q", "")
		test("a/b;mtp1=1;  q", "a/b;mtp1=1;  q", "")
		test("a/b;mtp1=1;  q=", "a/b;mtp1=1;  q=", "")
	})
}

func TestAcceptContentEncoding(t *testing.T) {
	t.Parallel()
	ftt.Run("Accept-Encoding", t, func(t *ftt.Test) {
		h := http.Header{}
		t.Run(`Empty`, func(t *ftt.Test) {
			ok, err := acceptsGZipResponse(h)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeFalse)
		})

		t.Run(`gzip`, func(t *ftt.Test) {
			h.Set("Accept-Encoding", "gzip")
			ok, err := acceptsGZipResponse(h)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run(`multiple values`, func(t *ftt.Test) {
			h.Add("Accept-Encoding", "gzip, deflate, br")
			ok, err := acceptsGZipResponse(h)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ok, should.BeTrue)
		})

		t.Run(`invalid input`, func(t *ftt.Test) {
			h.Add("Accept-Encoding", "gzip; q=a, deflate, br")
			_, err := acceptsGZipResponse(h)
			assert.Loosely(t, err, should.ErrLike("q parameter: expected a floating-point number"))
		})
	})
}
