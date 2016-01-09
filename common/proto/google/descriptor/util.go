// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package descriptor

// FindFile searches for a FileDescriptorProto by name.
func (s *FileDescriptorSet) FindFile(name string) *FileDescriptorProto {
	for _, x := range s.GetFile() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindService searches for a ServiceDescriptorProto by name.
func (p *FileDescriptorProto) FindService(name string) *ServiceDescriptorProto {
	for _, x := range p.GetService() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindMessage searches for a DescriptorProto by name.
func (p *FileDescriptorProto) FindMessage(name string) *DescriptorProto {
	for _, x := range p.GetMessageType() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindEnum searches for a EnumDescriptorProto by name.
func (p *FileDescriptorProto) FindEnum(name string) *EnumDescriptorProto {
	for _, x := range p.GetEnumType() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindMethod searches for a MethodDescriptorProto by name.
func (s *ServiceDescriptorProto) FindMethod(name string) *MethodDescriptorProto {
	for _, x := range s.GetMethod() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}

// FindField searches for a FieldDescriptorProto by name.
func (s *DescriptorProto) FindField(name string) *FieldDescriptorProto {
	for _, x := range s.GetField() {
		if x.GetName() == name {
			return x
		}
	}
	return nil
}
