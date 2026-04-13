// Copyright 2026 The LUCI Authors.
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

package value

import (
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

func TestPickData(t *testing.T) {
	t.Parallel()

	mkBin := func(val string) *orchestratorpb.ValueData {
		ret := &orchestratorpb.ValueData{}
		ret.SetBinary(&anypb.Any{TypeUrl: "binary", Value: []byte(val)})
		return ret
	}
	mkJson := func(val string) *orchestratorpb.ValueData {
		ret := &orchestratorpb.ValueData{}
		ret.SetJson(orchestratorpb.ValueData_JsonAny_builder{
			TypeUrl: proto.String("json"),
			Value:   proto.String(val),
		}.Build())
		return ret
	}
	withFail := func(d *orchestratorpb.ValueData, fail orchestratorpb.DataConversionFailure) *orchestratorpb.ValueData {
		ret := proto.Clone(d).(*orchestratorpb.ValueData)
		ret.SetConversionFailure(fail)
		return ret
	}

	t.Run("a is nil", func(t *testing.T) {
		b := mkBin("b")
		ret := PickData(nil, b)
		assert.That(t, ret, should.Equal(b))
	})

	t.Run("no-op same", func(t *testing.T) {
		a := mkBin("a")
		ret := PickData(a, a)
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("binary + json -> json", func(t *testing.T) {
		a := mkBin("a")
		b := mkJson("b")
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(b))
	})

	t.Run("binary + binary -> no-op", func(t *testing.T) {
		a := mkBin("a")
		b := mkBin("b")
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("json + binary -> no-op", func(t *testing.T) {
		a := mkJson("a")
		b := mkBin("b")
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("json + json -> no-op", func(t *testing.T) {
		a := mkJson("a")
		b := mkJson("b")
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("json + failure -> json", func(t *testing.T) {
		a := mkJson("a")
		b := withFail(mkBin("a"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("failure merge", func(t *testing.T) {
		a := mkBin("a")
		b := withFail(mkBin("b"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		ret := PickData(a, b)
		// * failure: unset -> set
		assert.That(t, ret, should.Equal(b))
	})

	t.Run("existing failure -> no-op", func(t *testing.T) {
		a := withFail(mkBin("a"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		b := withFail(mkBin("b"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_NO_DESCRIPTOR)
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("binary w/ failure + json -> json w/o failure", func(t *testing.T) {
		a := withFail(mkBin("a"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		b := mkJson("b")
		ret := PickData(a, b)
		assert.That(t, ret, should.Equal(b))
	})
}
