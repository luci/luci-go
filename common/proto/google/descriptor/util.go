// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package descriptor

import (
	"strings"
)

const (
	// NumberFileDescriptorProto_Package is the number of package field
	// in FileDescriptorProto message.
	NumberFileDescriptorProto_Package = 2
	// NumberFileDescriptorProto_Message is the number of message field
	// in FileDescriptorProto message.
	NumberFileDescriptorProto_Message = 4
	// NumberFileDescriptorProto_Enum is the number of enum field
	// in FileDescriptorProto message.
	NumberFileDescriptorProto_Enum = 5
	// NumberFileDescriptorProto_Service is the number of service field
	// in FileDescriptorProto message.
	NumberFileDescriptorProto_Service = 6

	// NumberServiceDescriptorProto_Method is the number of method field
	// in ServiceDescriptorProto message.
	NumberServiceDescriptorProto_Method = 2

	// NumberDescriptorProto_Field is the number of field field
	// in DescriptorProto message.
	NumberDescriptorProto_Field = 2
	// NumberDescriptorProto_NestedType is the number of nested_type field
	// in DescriptorProto message.
	NumberDescriptorProto_NestedType = 3
	// NumberDescriptorProto_EnumType is the number of enum_type field
	// in DescriptorProto message.
	NumberDescriptorProto_EnumType = 4
	// NumberDescriptorProto_OneOf is the number of oneof_decl field
	// in DescriptorProto message.
	NumberDescriptorProto_OneOf = 8

	// NumberEnumDescriptorProto_Value is the number of value field
	// in EnumDescriptorProto message.
	NumberEnumDescriptorProto_Value = 2
)

// splitFullName splits package name and service/type name.
func splitFullName(fullName string) (pkg string, name string) {
	lastDot := strings.LastIndex(fullName, ".")
	if lastDot < 0 {
		return "", fullName
	}
	return fullName[:lastDot], fullName[lastDot+1:]
}

////////////////////////////////////////////////////////////////////////////////
// FileDescriptorSet

// FindFile searches for a FileDescriptorProto by name.
func (s *FileDescriptorSet) FindFile(name string) int {
	for i, x := range s.GetFile() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// Resolve searches for an object by full name. obj can be one of
// *ServiceDescriptorProto,
// *MethodDescriptorProto,
// *DescriptorProto,
// *FieldDescriptorProto,
// *DescriptorProto,
// *EnumDescriptorProto,
// *EnumValueDescriptorProto or
// nil
//
// For path, see comment in SourceCodeInfo message.
func (s *FileDescriptorSet) Resolve(fullName string) (file *FileDescriptorProto, obj interface{}, path []int) {
	if fullName == "" {
		return nil, nil, nil
	}
	pkg, name := splitFullName(fullName)

	// Check top-level objects.
	for _, f := range s.GetFile() {
		if f.GetPackage() == pkg {
			if i := f.FindService(name); i != -1 {
				return f, f.Service[i], []int{NumberFileDescriptorProto_Service, i}
			}
			if i := f.FindMessage(name); i != -1 {
				return f, f.MessageType[i], []int{NumberFileDescriptorProto_Message, i}
			}
			if i := f.FindEnum(name); i != -1 {
				return f, f.EnumType[i], []int{NumberFileDescriptorProto_Enum, i}
			}
		}
	}

	// Recurse.
	var parent interface{}
	file, parent, path = s.Resolve(pkg)
	switch parent := parent.(type) {

	case *ServiceDescriptorProto:
		if i := parent.FindMethod(name); i != -1 {
			return file, parent.Method[i], append(path, NumberServiceDescriptorProto_Method, i)
		}

	case *DescriptorProto:
		if i := parent.FindMessage(name); i != -1 {
			return file, parent.NestedType[i], append(path, NumberDescriptorProto_NestedType, i)
		}
		if i := parent.FindEnum(name); i != -1 {
			return file, parent.EnumType[i], append(path, NumberDescriptorProto_EnumType, i)
		}
		if i := parent.FindField(name); i != -1 {
			return file, parent.Field[i], append(path, NumberDescriptorProto_Field, i)
		}
		if i := parent.FindOneOf(name); i != -1 {
			return file, parent.OneofDecl[i], append(path, NumberDescriptorProto_OneOf, i)
		}

	case *EnumDescriptorProto:
		if i := parent.FindValue(name); i != -1 {
			return file, parent.Value[i], append(path, NumberEnumDescriptorProto_Value, i)
		}
	}

	return nil, nil, nil
}

// FindService searches for a service by full name.
func (s *FileDescriptorSet) FindService(fullName string) (file *FileDescriptorProto, serviceIndex int) {
	pkg, name := splitFullName(fullName)
	for _, f := range s.GetFile() {
		if f.GetPackage() == pkg {
			if i := f.FindService(name); i != -1 {
				return f, i
			}
		}
	}
	return nil, -1
}

////////////////////////////////////////////////////////////////////////////////
// FileDescriptorProto

// FindService searches for a ServiceDescriptorProto by name.
func (f *FileDescriptorProto) FindService(name string) int {
	for i, x := range f.GetService() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// FindMessage searches for a DescriptorProto by name.
func (f *FileDescriptorProto) FindMessage(name string) int {
	for i, x := range f.GetMessageType() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// FindEnum searches for an EnumDescriptorProto by name.
func (f *FileDescriptorProto) FindEnum(name string) int {
	for i, x := range f.GetEnumType() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

////////////////////////////////////////////////////////////////////////////////
// ServiceDescriptorProto

// FindMethod searches for a MethodDescriptorProto by name.
func (s *ServiceDescriptorProto) FindMethod(name string) int {
	for i, x := range s.GetMethod() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

////////////////////////////////////////////////////////////////////////////////
// DescriptorProto (a message)

// FindField searches for a FieldDescriptorProto by name.
func (d *DescriptorProto) FindField(name string) int {
	for i, x := range d.GetField() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// FindMessage searches for a nested DescriptorProto by name.
func (d *DescriptorProto) FindMessage(name string) int {
	for i, x := range d.GetNestedType() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// FindEnum searches for a nested EnumDescriptorProto by name.
func (d *DescriptorProto) FindEnum(name string) int {
	for i, x := range d.GetEnumType() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// FindOneOf searches for a nested OneofDescriptorProto by name.
func (d *DescriptorProto) FindOneOf(name string) int {
	for i, x := range d.GetOneofDecl() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

////////////////////////////////////////////////////////////////////////////////
// FieldDescriptorProto

// Repeated returns true if the field is repeated.
func (f *FieldDescriptorProto) Repeated() bool {
	return f.GetLabel() == FieldDescriptorProto_LABEL_REPEATED
}

////////////////////////////////////////////////////////////////////////////////
// EnumDescriptorProto

// FindValue searches for an EnumValueDescriptorProto by name.
func (e *EnumDescriptorProto) FindValue(name string) int {
	for i, x := range e.GetValue() {
		if x.GetName() == name {
			return i
		}
	}
	return -1
}

// FindValueByNumber searches for an EnumValueDescriptorProto by number.
func (e *EnumDescriptorProto) FindValueByNumber(number int32) int {
	for i, x := range e.GetValue() {
		if x.GetNumber() == number {
			return i
		}
	}
	return -1
}

////////////////////////////////////////////////////////////////////////////////
// SourceCodeInfo

// FindLocation searches for a location by path.
func (s *SourceCodeInfo) FindLocation(path []int) *SourceCodeInfo_Location {
Outer:
	for _, l := range s.GetLocation() {
		if len(path) != len(l.Path) {
			continue
		}
		for i := range path {
			if int32(path[i]) != l.Path[i] {
				continue Outer
			}
		}
		return l
	}
	return nil
}
