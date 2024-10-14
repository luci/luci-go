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
	"context"
	"strconv"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/info"
)

type fakeInfo struct{ info.RawInterface }

func (fakeInfo) GetNamespace() string        { return "ns" }
func (fakeInfo) AppID() string               { return "aid" }
func (fakeInfo) FullyQualifiedAppID() string { return "s~aid" }

type fakeService struct{ RawInterface }

type fakeFilt struct{ RawInterface }

func (f fakeService) DecodeCursor(s string) (Cursor, error) {
	v, err := strconv.Atoi(s)
	return fakeCursor(v), err
}

func (f fakeService) Constraints() Constraints { return Constraints{} }

type fakeCursor int

func (f fakeCursor) String() string {
	return strconv.Itoa(int(f))
}

func TestServices(t *testing.T) {
	t.Parallel()

	ftt.Run("Test service interfaces", t, func(t *ftt.Test) {
		c := context.Background()
		t.Run("without adding anything", func(t *ftt.Test) {
			assert.Loosely(t, Raw(c), should.BeNil)
		})

		t.Run("adding a basic implementation", func(t *ftt.Test) {
			c = SetRaw(info.Set(c, fakeInfo{}), fakeService{})

			t.Run("and lets you add filters", func(t *ftt.Test) {
				c = AddRawFilters(c, func(ic context.Context, rds RawInterface) RawInterface {
					return fakeFilt{rds}
				})

				curs, err := DecodeCursor(c, "123")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, curs.String(), should.Equal("123"))
			})
		})
		t.Run("adding zero filters does nothing", func(t *ftt.Test) {
			assert.Loosely(t, AddRawFilters(c), should.Resemble(c))
		})
	})
}
