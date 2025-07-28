// Copyright 2023 The LUCI Authors.
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

package data

import (
	"bytes"
	"testing"

	"golang.org/x/exp/slices"
)

// okToType checks that a type conversion succeeds.
func okToType[T comparable](t *testing.T, input any, expect T) {
	t.Helper()
	if val, ok := LosslessConvertTo[T](input); !ok || val != expect {
		t.Errorf("%[1]T(%[1]v) - ok=%[2]t, value=%[3]T(%[3]v) | expect=%[4]v", input, ok, val, expect)
	}
}

// failToType checks that a type conversion fails.
func failToType[T comparable](t *testing.T, input any) {
	t.Helper()
	if val, ok := LosslessConvertTo[T](input); ok {
		t.Errorf("%[1]T(%[1]v) - ok=%[2]t, value=%[3]T(%[3]v)", input, ok, val)
	}
}

// TestConvertToInt tests conversion to signed integral types.
func TestConvertToInt(t *testing.T) {
	t.Parallel()

	// The integer 100 can be represented as an int8.
	//
	// However, we want to allow conversions to succeed only when **any** value of the given type would be possible to convert.
	// Only an int8 can cast losslessly to an int8.
	failToType[int8](t, int(100))
	okToType[int8](t, int8(100), 100)
	failToType[int8](t, int16(100))
	failToType[int8](t, int32(100))
	failToType[int8](t, int64(100))

	// An int8 or an int16 can cast losslessly to an int16
	failToType[int16](t, int(100))
	okToType[int16](t, int8(100), 100)
	okToType[int16](t, int16(100), 100)
	failToType[int16](t, int32(100))
	failToType[int16](t, int64(100))

	// An int8, int16, or int32 can cast losslessly to an int32.
	failToType[int32](t, int(100))
	okToType[int32](t, int8(100), 100)
	okToType[int32](t, int16(100), 100)
	okToType[int32](t, int32(100), 100)
	failToType[int32](t, int64(100))

	// An int8, int16, int32, or int64 can cast losslessly to an int64.
	okToType[int64](t, int(100), 100)
	okToType[int64](t, int8(100), 100)
	okToType[int64](t, int16(100), 100)
	okToType[int64](t, int32(100), 100)
	okToType[int64](t, int64(100), 100)

	// Can convert uint of a smaller bit size
	okToType[int64](t, uint16(100), 100)

	// all other coversions fail
	failToType[int](t, "no")
	failToType[int](t, nil)
	failToType[int](t, &struct{}{})
}

// TestConvertToInt tests conversion to unsigned integral types.
func TestConvertToUint(t *testing.T) {
	// only uint8 casts losslessly to uint8
	failToType[uint8](t, uint(100))
	okToType[uint8](t, uint8(100), 100)
	failToType[uint8](t, uint16(100))
	failToType[uint8](t, uint32(100))
	failToType[uint8](t, uint64(100))

	// only uint8 and uint16 casts losslessly to uint16
	failToType[uint16](t, uint(100))
	okToType[uint16](t, uint8(100), 100)
	okToType[uint16](t, uint16(100), 100)
	failToType[uint16](t, uint32(100))
	failToType[uint16](t, uint64(100))

	// only uint8, uint16, and uint32 casts losslessly to uint32
	failToType[uint32](t, uint(100))
	okToType[uint32](t, uint8(100), 100)
	okToType[uint32](t, uint16(100), 100)
	okToType[uint32](t, uint32(100), 100)
	failToType[uint32](t, uint64(100))

	// All unsigned integral types cast losslessly to uint32
	okToType[uint64](t, uint(100), 100)
	okToType[uint64](t, uint8(100), 100)
	okToType[uint64](t, uint16(100), 100)
	okToType[uint64](t, uint32(100), 100)
	okToType[uint64](t, uint64(100), 100)

	// Can not convert int of any size to uint
	failToType[uint64](t, int8(100))

	// all other coversions fail
	failToType[uint](t, "no")
	failToType[uint](t, nil)
	failToType[uint](t, &struct{}{})
}

// TestConvertToFloat tests conversions between float32 and float64.
func TestConvertToFloat(t *testing.T) {
	okToType[float32](t, float32(100.0), 100)
	okToType[float64](t, float32(100.0), 100)
	okToType[float64](t, float64(100.0), 100)
	failToType[float32](t, float64(100.0))

	// An int8 can be represented losslessly as a float.
	// However, we are going to be conservative and prevent this cast.
	//
	// I hope this decision doesn't come back to bite us.
	failToType[float32](t, int8(100))

	// All other coversions fail.
	failToType[float32](t, "no")
	failToType[float32](t, nil)
	failToType[float32](t, &struct{}{})
}

// TestConvertToComplex tests converting numbers to complex numbers.
func TestConvertToComplex(t *testing.T) {
	t.Parallel()

	// We allow conversions between complex64 and complex128 except for complex128->complex64.
	okToType[complex64](t, complex64(100.0), 100)
	okToType[complex128](t, complex64(100.0), 100)
	okToType[complex128](t, complex128(100.0), 100)
	failToType[complex64](t, complex128(100.0))

	// All other coversions fail.
	failToType[complex64](t, "no")
	failToType[complex64](t, int8(1))
	failToType[complex64](t, float32(1))
	failToType[complex64](t, nil)
	failToType[complex64](t, &struct{}{})
}

// TestConvertStrings tests conversion to strings.
func TestConvertStrings(t *testing.T) {
	t.Parallel()
	okToType[string](t, "hello", "hello")
	okToType[string](t, []byte("hello"), "hello")
	okToType[string](t, []rune("hello"), "hello")

	fail := func(message string, ok bool, val any, input any) {
		t.Errorf("%[1]T(%[1]v) - ok=%[2]t, value=%[3]T(%[3]v) | expect=%[4]v", message, ok, val, input)
	}

	if val, ok := LosslessConvertTo[[]byte]("hello"); !ok || !bytes.Equal(val, []byte("hello")) {
		fail("hello", ok, val, []byte("hello"))
	}

	if val, ok := LosslessConvertTo[[]rune]("hello"); !ok || !slices.Equal(val, []rune("hello")) {
		fail("hello", ok, val, []rune("hello"))
	}

	// reflect.Value.ConvertTo would allow this, but we do not.
	failToType[string](t, 100)
}

// TestNilConversion tests conversion of nil values.
func TestNilConversion(t *testing.T) {
	t.Parallel()
	okToType[any](t, nil, nil)
	okToType[*struct{}](t, nil, nil)

	if val, ok := LosslessConvertTo[func()](nil); !ok || val != nil {
		t.Errorf("%[1]T(%[1]v) - ok=%[2]t, value=%[3]T(%[3]v) | expect=%[4]v", "hello", ok, any(val), []byte("hello"))
	}
}

// TestInterfaceConversion tests converting a value to an interface that it satisfies.
func TestInterfaceConversion(t *testing.T) {
	t.Parallel()
	okToType[any](t, 100, 100)
}

// TestDifferentConcreteTypes tests casting between different conrete types with teh same underlying representation.
func TestDifferentConcreteTypes(t *testing.T) {
	t.Parallel()
	type myString string
	okToType[string](t, myString("hello"), "hello")
	okToType[myString](t, "hello", "hello")

	type myStruct struct{ CoolField int }
	okToType[myStruct](t, struct{ CoolField int }{100}, myStruct{100})
	okToType[myStruct](t, myStruct{100}, myStruct{100})
	okToType[struct{ CoolField int }](t, myStruct{100}, struct{ CoolField int }{100})

	if val, ok := LosslessConvertTo[*myStruct](&struct{ CoolField int }{100}); !ok || val == nil || val.CoolField != 100 {
		t.Error("failed to losslessly convert &struct{...} to *myStruct")
	}
}
