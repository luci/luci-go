// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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
