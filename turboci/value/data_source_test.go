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

func TestMergeData(t *testing.T) {
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
		delta, ret := MergeData(nil, b)
		assert.That(t, delta, should.Equal(int64(proto.Size(b))))
		assert.That(t, ret, should.Match(b))
	})

	t.Run("no-op same", func(t *testing.T) {
		a := mkBin("a")
		delta, ret := MergeData(a, a)
		assert.Loosely(t, delta, should.Equal(0))
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("binary + json -> json", func(t *testing.T) {
		a := mkBin("a")
		b := mkJson("b")
		delta, ret := MergeData(a, b)
		// * type_url: binary -> json
		assert.That(t, delta, should.Equal(int64(-2)))
		assert.That(t, ret.HasJson(), should.BeTrue)
		assert.That(t, ret.GetJson().GetValue(), should.Equal("b"))
	})

	t.Run("binary + binary -> no-op", func(t *testing.T) {
		a := mkBin("a")
		b := mkBin("b")
		delta, ret := MergeData(a, b)
		assert.Loosely(t, delta, should.Equal(0))
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("json + binary -> no-op", func(t *testing.T) {
		a := mkJson("a")
		b := mkBin("b")
		delta, ret := MergeData(a, b)
		assert.Loosely(t, delta, should.Equal(0))
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("json + json -> no-op", func(t *testing.T) {
		a := mkJson("a")
		b := mkJson("b")
		delta, ret := MergeData(a, b)
		assert.Loosely(t, delta, should.Equal(0))
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("failure merge", func(t *testing.T) {
		a := mkBin("a")
		b := withFail(mkBin("b"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		delta, ret := MergeData(a, b)
		// * failure: unset -> set
		assert.That(t, delta, should.Equal(int64(2)))
		assert.That(t, ret.GetConversionFailure(),
			should.Equal(orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR))
		assert.That(t, ret.HasBinary(), should.BeTrue)
	})

	t.Run("existing failure -> no-op", func(t *testing.T) {
		a := withFail(mkBin("a"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		b := withFail(mkBin("b"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_NO_DESCRIPTOR)
		delta, ret := MergeData(a, b)
		assert.Loosely(t, delta, should.Equal(0))
		assert.That(t, ret, should.Equal(a))
	})

	t.Run("binary w/ failure + json -> json w/o failure", func(t *testing.T) {
		a := withFail(mkBin("a"), orchestratorpb.DataConversionFailure_DATA_CONVERSION_FAILURE_ERROR)
		b := mkJson("b")
		delta, ret := MergeData(a, b)
		// * type url: binary -> json
		// * failure: set -> unset
		assert.That(t, delta, should.Equal(int64(-4)))
		assert.That(t, ret.HasJson(), should.BeTrue)
		assert.That(t, ret.HasConversionFailure(), should.BeFalse)
	})
}
