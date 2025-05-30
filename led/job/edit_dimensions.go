// Copyright 2020 The LUCI Authors.
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

package job

import (
	fmt "fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	durpb "google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

// ExpiringValue represents a tuple of dimension value, plus an expiration time.
//
// If Expiration is zero, it counts as "no expiration".
type ExpiringValue struct {
	Value      string
	Expiration time.Duration
}

// DimensionEditCommand is instruction on how to process the values in the task
// associated with a swarming dimension.
//
// The fields are processed in order:
//   - if SetValues is non-nil, the dimension values are set to this set
//     (including empty).
//   - if RemoveValues is non-empty, these values will be removed from the
//     dimension values.
//   - if AddValues is non-empty, these values will ber added to the dimension
//     values.
//
// If the set of values at the end of this process is empty, the dimension will
// be removed from the task. Otherwise the dimension will be set to the sorted
// remaining values.
type DimensionEditCommand struct {
	SetValues    []ExpiringValue
	RemoveValues []string
	AddValues    []ExpiringValue
}

// DimensionEditCommands is a mapping of dimension name to a set of commands to
// apply to the values of that dimension.
type DimensionEditCommands map[string]*DimensionEditCommand

func split2(s, sep string) (a, b string, ok bool) {
	idx := strings.Index(s, sep)
	if idx == -1 {
		return s, "", false
	}
	return s[:idx], s[idx+1:], true
}

func rsplit2(s, sep string) (a, b string) {
	idx := strings.LastIndex(s, sep)
	if idx == -1 {
		return s, ""
	}
	return s[:idx], s[idx+1:]
}

func parseDimensionEditCmd(cmd string) (dim, op, val string, exp time.Duration, err error) {
	dim, valueExp, ok := split2(cmd, "=")
	if !ok {
		err = errors.New("expected $key$op$value, but op was missing")
		return
	}

	switch dim[len(dim)-1] {
	case '-':
		op = "-="
		dim = dim[:len(dim)-1]
	case '+':
		op = "+="
		dim = dim[:len(dim)-1]
	default:
		op = "="
	}

	val, expStr := rsplit2(valueExp, "@")
	if expStr != "" {
		var expSec int
		if expSec, err = strconv.Atoi(expStr); err != nil {
			err = errors.Fmt("parsing expiration %q: %w", expStr, err)
			return
		}
		exp = time.Second * time.Duration(expSec)
	}

	if val == "" && op != "=" {
		err = errors.Fmt("empty value not allowed for operator %q: %q", op, cmd)
	}
	if exp != 0 && op == "-=" {
		err = errors.Fmt("expiration seconds not allowed for operator %q: %q", op, cmd)
	}

	return
}

// MakeDimensionEditCommands takes a slice of commands in the form of:
//
//	dimension=
//	dimension=value
//	dimension=value@1234
//
//	dimension-=value
//
//	dimension+=value
//	dimension+=value@1234
//
// Logically:
//   - dimension_name - The name of the dimension to modify
//   - operator
//   - "=" - Add value to SetValues. If empty, ensures that SetValues is
//     non-nil (i.e. clear all values for this dimension).
//   - "-=" - Add value to RemoveValues.
//   - "+=" - Add value to AddValues.
//   - value - The dimension value for the operand
//   - expiration seconds - The time at which this value should expire.
//
// All equivalent operations for the same dimension will be grouped into
// a single DimensionEditCommand in the order they appear in `commands`.
func MakeDimensionEditCommands(commands []string) (DimensionEditCommands, error) {
	if len(commands) == 0 {
		return nil, nil
	}

	ret := DimensionEditCommands{}
	for _, command := range commands {
		dimension, operator, value, expiration, err := parseDimensionEditCmd(command)
		if err != nil {
			return nil, errors.Fmt("parsing %q: %w", command, err)
		}
		editCmd := ret[dimension]
		if editCmd == nil {
			editCmd = &DimensionEditCommand{}
			ret[dimension] = editCmd
		}
		switch operator {
		case "=":
			// explicitly setting SetValues takes care of the 'dimension=' case.
			if editCmd.SetValues == nil {
				editCmd.SetValues = []ExpiringValue{}
			}
			if value != "" {
				editCmd.SetValues = append(editCmd.SetValues, ExpiringValue{
					Value: value, Expiration: expiration,
				})
			}
		case "-=":
			editCmd.RemoveValues = append(editCmd.RemoveValues, value)
		case "+=":
			editCmd.AddValues = append(editCmd.AddValues, ExpiringValue{
				Value: value, Expiration: expiration,
			})
		}
	}
	return ret, nil
}

// Applies the DimensionEditCommands to the given logicalDimensions.
func (dimEdits DimensionEditCommands) apply(dimMap logicalDimensions, minExp time.Duration) {
	if len(dimEdits) == 0 {
		return
	}

	shouldApply := func(eVal ExpiringValue) bool {
		return eVal.Expiration == 0 || minExp == 0 || eVal.Expiration >= minExp
	}

	for dim, edits := range dimEdits {
		if edits.SetValues != nil {
			dimMap[dim] = make(dimValueExpiration, len(edits.SetValues))
			for _, expVal := range edits.SetValues {
				if shouldApply(expVal) {
					dimMap[dim][expVal.Value] = expVal.Expiration
				}
			}
		}
		for _, value := range edits.RemoveValues {
			delete(dimMap[dim], value)
		}
		for _, expVal := range edits.AddValues {
			if shouldApply(expVal) {
				expValMap := dimMap[dim]
				if expValMap == nil {
					expValMap = dimValueExpiration{}
					dimMap[dim] = expValMap
				}
				expValMap[expVal.Value] = expVal.Expiration
			}
		}
	}
	toRemove := stringset.New(len(dimMap))
	for dim, valExps := range dimMap {
		if len(valExps) == 0 {
			toRemove.Add(dim)
		}
	}
	toRemove.Iter(func(dim string) bool {
		delete(dimMap, dim)
		return true
	})
}

// ExpiringDimensions is a map from dimension name to a list of values
// corresponding to that dimension.
//
// When retrieved from a led library, the values will be sorted by expiration
// time, followed by value. Expirations of 0 (i.e. "infinite") are sorted last.
type ExpiringDimensions map[string][]ExpiringValue

func (e ExpiringDimensions) String() string {
	bits := []string{}
	for key, values := range e {
		for _, value := range values {
			if value.Expiration == 0 {
				bits = append(bits, fmt.Sprintf("%s=%s", key, value.Value))
			} else {
				bits = append(bits, fmt.Sprintf(
					"%s=%s@%d", key, value.Value, value.Expiration/time.Second))
			}
		}
	}
	return strings.Join(bits, ", ")
}

func (e ExpiringDimensions) toLogical() logicalDimensions {
	ret := logicalDimensions{}
	for key, expVals := range e {
		dve := ret[key]
		if dve == nil {
			dve = dimValueExpiration{}
			ret[key] = dve
		}
		for _, expVal := range expVals {
			dve[expVal.Value] = expVal.Expiration
		}
	}
	return ret
}

type dimValueExpiration map[string]time.Duration

func (valExps dimValueExpiration) toSlice() []string {
	ret := make([]string, 0, len(valExps))
	for value := range valExps {
		ret = append(ret, value)
	}
	sort.Strings(ret)
	return ret
}

// A multimap of dimension to values.
type logicalDimensions map[string]dimValueExpiration

func (dims logicalDimensions) update(dim, value string, expiration *durpb.Duration) {
	var exp time.Duration
	if expiration != nil {
		var err error
		if exp, err = ptypes.Duration(expiration); err != nil {
			panic(err)
		}
	}
	dims.updateDuration(dim, value, exp)
}

func (dims logicalDimensions) updateDuration(dim, value string, exp time.Duration) {
	if dims[dim] == nil {
		dims[dim] = dimValueExpiration{}
	}
	dims[dim][value] = exp
}

func expLess(a, b time.Duration) bool {
	if a == 0 { // b is either infinity (==a) or finite (<a)
		return false
	} else if b == 0 { // b is infinity, a is finite
		return true
	}
	return a < b
}

func (dims logicalDimensions) toExpiringDimensions() ExpiringDimensions {
	ret := ExpiringDimensions{}
	for key, dve := range dims {
		for dim, expiration := range dve {
			ret[key] = append(ret[key], ExpiringValue{dim, expiration})
		}
		sort.Slice(ret[key], func(i, j int) bool {
			a, b := ret[key][i], ret[key][j]
			if a.Expiration == b.Expiration {
				return a.Value < b.Value
			}
			return expLess(a.Expiration, b.Expiration)
		})
	}
	return ret
}
