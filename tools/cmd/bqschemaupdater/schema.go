// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"fmt"
	"strings"
	"unicode"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google/descutil"
)

// sourceCodeInfoMap maps descriptor proto messages to source code info,
// if available.
// See also descutil.IndexSourceCodeInfo.
type sourceCodeInfoMap map[interface{}]*descriptor.SourceCodeInfo_Location

type schemaConverter struct {
	desc           *descriptor.FileDescriptorSet
	sourceCodeInfo map[*descriptor.FileDescriptorProto]sourceCodeInfoMap
}

// schema constructs a bigquery.Schema from a named message.
func (c *schemaConverter) schema(messageName string) (schema bigquery.Schema, description string, err error) {
	file, obj, _ := descutil.Resolve(c.desc, messageName)
	if obj == nil {
		return nil, "", fmt.Errorf("message %q is not found", messageName)
	}
	msg, isMsg := obj.(*descriptor.DescriptorProto)
	if !isMsg {
		return nil, "", fmt.Errorf("expected %q to be a message, but it is %T", messageName, obj)
	}

	s := make(bigquery.Schema, len(msg.Field))
	for i, field := range msg.Field {
		var err error
		s[i], err = c.field(file, field)
		if err != nil {
			return nil, "", errors.Annotate(err, "failed to derive schema for field %q in message %q", field.Name, msg.Name).Err()
		}
	}
	return s, c.description(file, msg), nil
}

// field constructs bigquery.FieldSchema from proto field descriptor.
func (c *schemaConverter) field(file *descriptor.FileDescriptorProto, field *descriptor.FieldDescriptorProto) (*bigquery.FieldSchema, error) {
	schema := &bigquery.FieldSchema{
		Name:        field.GetName(),
		Description: c.description(file, field),
		Repeated:    descutil.Repeated(field),
		Required:    descutil.Required(field),
	}

	typeName := strings.TrimPrefix(field.GetTypeName(), ".")
	switch field.GetType() {
	case
		descriptor.FieldDescriptorProto_TYPE_DOUBLE,
		descriptor.FieldDescriptorProto_TYPE_FLOAT:

		schema.Type = bigquery.FloatFieldType

	case
		descriptor.FieldDescriptorProto_TYPE_INT64,
		descriptor.FieldDescriptorProto_TYPE_UINT64,
		descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_FIXED64,
		descriptor.FieldDescriptorProto_TYPE_FIXED32,
		descriptor.FieldDescriptorProto_TYPE_UINT32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_SINT32,
		descriptor.FieldDescriptorProto_TYPE_SINT64:

		schema.Type = bigquery.IntegerFieldType

	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		schema.Type = bigquery.BooleanFieldType

	case descriptor.FieldDescriptorProto_TYPE_STRING:
		schema.Type = bigquery.StringFieldType

	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		schema.Type = bigquery.BytesFieldType

	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		schema.Type = bigquery.StringFieldType

	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		if typeName == "google.protobuf.Timestamp" {
			schema.Type = bigquery.TimestampFieldType
		} else {
			schema.Type = bigquery.RecordFieldType
			var err error
			if schema.Schema, _, err = c.schema(typeName); err != nil {
				return nil, err
			}
		}
	default:
		return nil, fmt.Errorf("not supported field type %q", field.GetType())
	}
	return schema, nil
}

// description returns a string description of the descriptor proto that
// ptr points to.
// If ptr is a field of an enum type, appends
// "\nValid values: <comma-separated enum member names>".
func (c *schemaConverter) description(file *descriptor.FileDescriptorProto, ptr interface{}) string {
	description := c.sourceCodeInfo[file][ptr].GetLeadingComments()

	// Trim leading whitespace.
	lines := strings.Split(description, "\n")
	trimSize := -1
	for _, l := range lines {
		if len(strings.TrimSpace(l)) == 0 {
			// skip empty lines
			continue
		}
		space := 0
		for _, r := range l {
			if unicode.IsSpace(r) {
				space++
			} else {
				break
			}
		}
		if trimSize == -1 || space < trimSize {
			trimSize = space
		}
	}
	if trimSize > 0 {
		for i := range lines {
			if len(lines[i]) >= trimSize {
				lines[i] = lines[i][trimSize:]
			}
		}
		description = strings.Join(lines, "\n")
	}
	description = strings.TrimSpace(description)

	// Append valid enum values.
	if field, ok := ptr.(*descriptor.FieldDescriptorProto); ok && field.GetType() == descriptor.FieldDescriptorProto_TYPE_ENUM {
		_, obj, _ := descutil.Resolve(c.desc, strings.TrimPrefix(field.GetTypeName(), "."))
		if enum, ok := obj.(*descriptor.EnumDescriptorProto); ok {
			names := make([]string, len(enum.Value))
			for i, v := range enum.Value {
				names[i] = v.GetName()
			}
			if description != "" {
				description += "\n"
			}
			description += fmt.Sprintf("Valid values: %s.", strings.Join(names, ", "))
		}
	}
	return description
}
