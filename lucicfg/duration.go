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

package lucicfg

import (
	"fmt"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"
)

var zero = starlark.MakeInt64(0)

// duration wraps an integer, making it a distinct integer-like type.
type duration struct {
	starlark.Int // milliseconds
}

// Type returns 'duration', to make the type different from ints.
func (x duration) Type() string {
	return "duration"
}

// String formats the duration using Go's time.Duration rules.
func (x duration) String() string {
	ms, ok := x.Int64()
	if !ok {
		return "<invalid-duration>" // probably very-very large
	}
	return (time.Duration(ms) * time.Millisecond).String()
}

// Cmp makes durations comparable by comparing them as integers.
func (x duration) Cmp(y starlark.Value, depth int) (int, error) {
	return x.Int.Cmp(y.(duration).Int, depth)
}

// Binary implements binary operations between durations and ints.
func (x duration) Binary(op syntax.Token, y starlark.Value, side starlark.Side) (starlark.Value, error) {
	switch y := y.(type) {
	case starlark.Int:
		switch {
		case op == syntax.STAR:
			return duration{x.Int.Mul(y)}, nil
		case (op == syntax.SLASH || op == syntax.SLASHSLASH) && side == starlark.Left:
			return duration{x.Int.Div(y)}, nil
		}

	case duration:
		switch {
		case op == syntax.PLUS:
			return duration{x.Int.Add(y.Int)}, nil
		case op == syntax.MINUS && side == starlark.Left:
			return duration{x.Int.Sub(y.Int)}, nil
		case op == syntax.MINUS && side == starlark.Right:
			return duration{y.Int.Sub(x.Int)}, nil
		case (op == syntax.SLASH || op == syntax.SLASHSLASH) && side == starlark.Left:
			return x.Int.Div(y.Int), nil
		case (op == syntax.SLASH || op == syntax.SLASHSLASH) && side == starlark.Right:
			return y.Int.Div(x.Int), nil
		case (op == syntax.PERCENT) && side == starlark.Left:
			return x.Int.Mod(y.Int), nil
		case (op == syntax.PERCENT) && side == starlark.Right:
			return y.Int.Mod(x.Int), nil
		}
	}

	// All other combinations aren't supported.
	return nil, nil
}

// Unary implements +-.
func (x duration) Unary(op syntax.Token) (starlark.Value, error) {
	switch op {
	case syntax.PLUS:
		return x, nil
	case syntax.MINUS:
		return duration{zero.Sub(x.Int)}, nil
	}
	return nil, nil
}

func init() {
	// make_duration(milliseconds) returns a 'duration' value.
	declNative("make_duration", func(call nativeCall) (starlark.Value, error) {
		var ms starlark.Int
		if err := call.unpack(0, &ms); err != nil {
			return nil, err
		}
		return duration{ms}, nil
	})

	// epoch(layout, value, location) returns int epoch seconds for value parsed as a time per layout in location.
	declNative("epoch", func(call nativeCall) (starlark.Value, error) {
		var layout starlark.String
		var value starlark.String
		var location starlark.String
		if err := call.unpack(3, &layout, &value, &location); err != nil {
			return nil, err
		}
		loc, err := time.LoadLocation(location.GoString())
		if err != nil {
			return nil, fmt.Errorf("time.epoch: %s", err)
		}
		t, err := time.ParseInLocation(layout.GoString(), value.GoString(), loc)
		if err != nil {
			return nil, fmt.Errorf("time.epoch: %s", err)
		}
		return starlark.MakeInt(int(t.UnixNano() / 1000000000)), nil
	})
}
