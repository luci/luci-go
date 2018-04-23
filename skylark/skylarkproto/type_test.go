// Copyright 2018 The LUCI Authors.
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

package skylarkproto

import (
	"reflect"
	"testing"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/luci/skylark/skylarkproto/testprotos"
)

func TestGetMessageType(t *testing.T) {
	t.Parallel()

	mt, err := GetMessageType(proto.MessageType("testprotos.Complex"))
	if err != nil {
		t.Fatalf("unexpected - %s", err)
	}

	if mt.Name() != "testprotos.Complex" {
		t.Fatalf("bad name %q", mt.Name())
	}
	if typ := reflect.TypeOf(&testprotos.Complex{}); mt.Type() != typ {
		t.Fatalf("expected %s, got %s", typ, mt.Type())
	}

	// Getting same type again returns exact same object.
	mt2, _ := GetMessageType(proto.MessageType("testprotos.Complex"))
	if mt != mt2 {
		t.FailNow()
	}

	// Discovered all fields.
	expected := []string{
		"enum_val",
		"i64",
		"i64_rep",
		"msg_val",
		"oneof_val", // TODO: this is wrong and will be fixed with oneof support
	}
	if !reflect.DeepEqual(mt.fieldNames, expected) {
		t.Fatalf("%v != %s", mt.fieldNames, expected)
	}

	// Types and getters for fields are correct.
	msg := testprotos.Complex{
		EnumVal: testprotos.Complex_ENUM_VAL_1,
		I64:     123,
		I64Rep:  []int64{1, 2, 3},
		MsgVal:  &testprotos.Complex_InnerMessage{I: 456},
		// TODO: add a test for oneof once supported
	}
	val := reflect.ValueOf(msg)

	var desc fieldDesc
	var v interface{}

	// Copy-pasta below to avoid using reflection for testing reflection to reduce
	// chances of making identical self-canceling mistakes in tests and code under
	// test.

	desc = mt.fields["enum_val"]
	if desc.typ != reflect.TypeOf(msg.EnumVal) {
		t.Errorf("got bad type %s", desc.typ)
	}
	v = desc.value(val).Interface()
	if v.(testprotos.Complex_InnerEnum) != msg.EnumVal {
		t.Errorf("bad getter, returned incorrect value - %v", v)
	}

	desc = mt.fields["i64"]
	if desc.typ != reflect.TypeOf(msg.I64) {
		t.Errorf("got bad type %s", desc.typ)
	}
	v = desc.value(val).Interface()
	if v.(int64) != msg.I64 {
		t.Errorf("bad getter, returned incorrect value - %v", v)
	}

	desc = mt.fields["i64_rep"]
	if desc.typ != reflect.TypeOf(msg.I64Rep) {
		t.Errorf("got bad type %s", desc.typ)
	}
	v = desc.value(val).Interface()
	if !reflect.DeepEqual(v.([]int64), msg.I64Rep) {
		t.Errorf("bad getter, returned incorrect value - %v", v)
	}

	desc = mt.fields["msg_val"]
	if desc.typ != reflect.TypeOf(msg.MsgVal) {
		t.Errorf("got bad type %s", desc.typ)
	}
	v = desc.value(val).Interface()
	if v.(*testprotos.Complex_InnerMessage) != msg.MsgVal { // exact same pointer
		t.Errorf("bad getter, returned incorrect value - %v", v)
	}

	// TODO: add oneof tests.
}
