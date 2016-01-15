// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package descriptor

import "strings"

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
func (s *FileDescriptorSet) FindFile(name string) *FileDescriptorProto {
	for _, x := range s.GetFile() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindMessage searches for a message by full name.
func (s *FileDescriptorSet) FindMessage(fullName string) *DescriptorProto {
	pkg, name := splitFullName(fullName)
	for _, f := range s.GetFile() {
		if f.GetPackage() == pkg {
			if x := f.FindMessage(name); x != nil {
				return x
			}
		}
	}
	return nil
}

// FindEnum searches for a enum by full name.
func (s *FileDescriptorSet) FindEnum(fullName string) *EnumDescriptorProto {
	pkg, name := splitFullName(fullName)
	for _, f := range s.GetFile() {
		if f.GetPackage() == pkg {
			if x := f.FindEnum(name); x != nil {
				return x
			}
		}
	}
	return nil
}

// FindService searches for a service by full name.
func (s *FileDescriptorSet) FindService(fullName string) *ServiceDescriptorProto {
	pkg, name := splitFullName(fullName)
	for _, f := range s.GetFile() {
		if f.GetPackage() == pkg {
			if x := f.FindService(name); x != nil {
				return x
			}
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// FileDescriptorProto

// FindService searches for a ServiceDescriptorProto by name.
func (f *FileDescriptorProto) FindService(name string) *ServiceDescriptorProto {
	for _, x := range f.GetService() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindMessage searches for a DescriptorProto by name.
func (f *FileDescriptorProto) FindMessage(name string) *DescriptorProto {
	for _, x := range f.GetMessageType() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindEnum searches for a EnumDescriptorProto by name.
func (f *FileDescriptorProto) FindEnum(name string) *EnumDescriptorProto {
	for _, x := range f.GetEnumType() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// ServiceDescriptorProto

// FindMethod searches for a MethodDescriptorProto by name.
func (s *ServiceDescriptorProto) FindMethod(name string) *MethodDescriptorProto {
	for _, x := range s.GetMethod() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// DescriptorProto (a message)

// FindField searches for a FieldDescriptorProto by name.
func (d *DescriptorProto) FindField(name string) *FieldDescriptorProto {
	for _, x := range d.GetField() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////
// SourceCodeInfo

// FindLocation searches for a location by path.
func (s *SourceCodeInfo) FindLocation(path []int) *SourceCodeInfo_Location {
Outer:
	for _, l := range s.Location {
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
