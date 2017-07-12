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

package field

import (
	"fmt"
)

// Field is the definition of a metric field.  It has a name and a type
// (string, int or bool).
type Field struct {
	Name string
	Type Type
}

type Type int

const (
	StringType Type = iota + 1
	IntType
	BoolType
)

func (f Field) String() string {
	return fmt.Sprintf("Field(%s, %s)", f.Name, f.Type)
}

func (t Type) String() string {
	switch t {
	case StringType:
		return "String"
	case IntType:
		return "Int"
	case BoolType:
		return "Bool"
	}
	panic(fmt.Sprintf("unknown field.Type %d", t))
}

// String returns a new string-typed field.
func String(name string) Field { return Field{name, StringType} }

// Bool returns a new bool-typed field.
func Bool(name string) Field { return Field{name, BoolType} }

// Int returns a new int-typed field.  Internally values for these fields are
// stored as int64s.
func Int(name string) Field { return Field{name, IntType} }
