// Copyright 2024 The LUCI Authors.
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

package results

import (
	"reflect"
)

// A ResultBuilder builds a Result and has a fluent interface.
type ResultBuilder struct {
	result *OldResult
}

// NewResultBuilder returns an empty ResultBuilder.
func NewResultBuilder() ResultBuilder {
	return ResultBuilder{}
}

// Result returns the finished result from a builder.
func (builder ResultBuilder) Result() *OldResult {
	return builder.result
}

// SetName sets the name of a Result.
func (builder ResultBuilder) SetName(comparison string, types ...reflect.Type) ResultBuilder {
	if builder.result == nil {
		builder.result = &OldResult{}
	}
	builder.result.failed = true
	var typeNames []string
	for _, typ := range types {
		if typ == nil {
			typeNames = append(typeNames, "<nil>")
			continue
		}
		typeNames = append(typeNames, typ.String())
	}
	builder.result.header.comparison = comparison
	builder.result.header.types = typeNames
	return builder
}

// Because sets the because field of a Result.
func (builder ResultBuilder) Because(format string, args ...interface{}) ResultBuilder {
	if builder.result == nil {
		builder.result = &OldResult{}
	}
	builder.result.failed = true
	addValuef(builder.result, "Because", format, args...)
	return builder
}

// Actual sets the actual field of a Result.
func (builder ResultBuilder) Actual(actual interface{}) ResultBuilder {
	if builder.result == nil {
		builder.result = &OldResult{}
	}
	builder.result.failed = true
	addValue(builder.result, "Actual", actual)
	return builder
}

// Expected sets the expected field of a Result.
func (builder ResultBuilder) Expected(actual interface{}) ResultBuilder {
	if builder.result == nil {
		builder.result = &OldResult{}
	}
	builder.result.failed = true
	addValue(builder.result, "Expected", actual)
	return builder
}
