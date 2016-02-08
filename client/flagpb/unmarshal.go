// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package flagpb

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/luci/luci-go/common/proto/google/descriptor"
)

// UnmarshalUntyped unmarshals a key-value map from flags
// using a protobuf message descriptor.
func UnmarshalUntyped(flags []string, desc *descriptor.DescriptorProto, resolver Resolver) (map[string]interface{}, error) {
	p := parser{resolver}
	return p.parse(flags, desc)
}

type message struct {
	data map[string]interface{}
	desc *descriptor.DescriptorProto
}

type parser struct {
	Resolver Resolver
}

func (p *parser) parse(flags []string, desc *descriptor.DescriptorProto) (map[string]interface{}, error) {
	if desc == nil {
		panic("desc is nil")
	}
	root := message{map[string]interface{}{}, desc}

	for len(flags) > 0 {
		var err error
		if flags, err = p.parseOneFlag(flags, root); err != nil {
			return nil, err
		}
	}
	return root.data, nil
}

func (p *parser) parseOneFlag(flags []string, root message) (flagsRest []string, err error) {
	// skip empty flags
	for len(flags) > 0 && strings.TrimSpace(flags[0]) == "" {
		flags = flags[1:]
	}
	if len(flags) == 0 {
		return flags, nil
	}

	firstArg := flags[0]
	flags = flags[1:]

	// Prefix returned errors with flag name verbatim.
	defer func() {
		if err != nil {
			err = fmt.Errorf("%s: %s", firstArg, err)
		}
	}()

	// Trim dashes.
	if !strings.HasPrefix(firstArg, "-") {
		return nil, fmt.Errorf("a flag was expected")
	}
	flagName := strings.TrimPrefix(firstArg, "-") // -foo
	flagName = strings.TrimPrefix(flagName, "-")  // --foo
	if strings.HasPrefix(flagName, "-") {
		// Triple dash is too much.
		return nil, fmt.Errorf("bad flag syntax")
	}

	// Split key-value pair x=y.
	flagName, value, hasValue := p.splitKeyValuePair(flagName)
	if flagName == "" {
		return nil, fmt.Errorf("bad flag syntax")
	}

	// Split field path "a.b.c" and resolve field names.
	fieldPath := strings.Split(flagName, ".")
	pathMsgs, err := p.subMessages(root, fieldPath[:len(fieldPath)-1])
	if err != nil {
		return nil, err
	}

	// Where to assign a value?
	target := &root
	if len(pathMsgs) > 0 {
		lastMsg := pathMsgs[len(pathMsgs)-1]
		target = &lastMsg.message
	}

	// Resolve last field name.
	name := fieldPath[len(fieldPath)-1]
	fi := target.desc.FindField(name)
	if fi == -1 {
		return nil, fmt.Errorf("field %s not found in message %s", name, target.desc.GetName())
	}
	field := target.desc.Field[fi]

	if !hasValue {
		switch {
		// Boolean and repeated message fields may have no value and ignore
		// next argument.
		case field.GetType() == descriptor.FieldDescriptorProto_TYPE_BOOL:
			target.data[name] = true
			return flags, nil
		case field.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE && field.Repeated():
			target.data[name] = append(asSlice(target.data[name]), map[string]interface{}{})
			return flags, nil

		// Read next argument as a value.
		default:
			if len(flags) == 0 {
				return nil, fmt.Errorf("value was expected")
			}
			value, flags = flags[0], flags[1:]
		}
	}

	// Check if the value is already set.
	if target.data[name] != nil && !field.Repeated() {
		repeatedFields := make([]string, 0, len(pathMsgs))
		for _, m := range pathMsgs {
			if m.repeated {
				repeatedFields = append(repeatedFields, "-"+strings.Join(m.path, "."))
			}
		}
		if len(repeatedFields) == 0 {
			return nil, fmt.Errorf("value is already set to %v", target.data[name])
		}
		return nil, fmt.Errorf(
			"value is already set to %v. Did you forgot to insert %s in between to declare a new repeated message?",
			target.data[name], strings.Join(repeatedFields, " or "))
	}

	// Parse the value and set/append it.
	parsedValue, err := p.parseFieldValue(value, target.desc.GetName(), field)
	if err != nil {
		return nil, err
	}

	if !field.Repeated() {
		target.data[name] = parsedValue
	} else {
		target.data[name] = append(asSlice(target.data[name]), parsedValue)
	}
	return flags, nil
}

type subMsg struct {
	message
	path     []string
	repeated bool
}

// subMessages returns message field values at each component of the path.
// For example, for path ["a", "b", "c"] it will return
// [msg.a, msg.a.b, msg.a.b.c].
// If a field is repeated, returns the last message.
//
// If a field value is nil, initializes it with an empty message or slice.
// If a field is not a message field, returns an error.
func (p *parser) subMessages(root message, path []string) ([]subMsg, error) {
	result := make([]subMsg, 0, len(path))

	parent := &root
	for i, name := range path {
		curPath := path[:i+1]
		fi := parent.desc.FindField(name)
		if fi == -1 {
			return nil, fmt.Errorf("field %q not found in message %s", name, parent.desc.GetName())
		}

		f := parent.desc.Field[fi]
		if f.GetType() != descriptor.FieldDescriptorProto_TYPE_MESSAGE {
			return nil, fmt.Errorf("field %s is not a message", strings.Join(curPath, "."))
		}

		subDescInterface, err := p.resolve(f.GetTypeName())
		if err != nil {
			return nil, err
		}
		subDesc, ok := subDescInterface.(*descriptor.DescriptorProto)
		if !ok {
			return nil, fmt.Errorf("%s is not a message", f.GetTypeName())
		}

		sub := subMsg{
			message:  message{desc: subDesc},
			repeated: f.Repeated(),
			path:     curPath,
		}
		if value, ok := parent.data[name]; !ok {
			sub.data = map[string]interface{}{}
			if sub.repeated {
				parent.data[name] = []interface{}{sub.data}
			} else {
				parent.data[name] = sub.data
			}
		} else {
			if sub.repeated {
				slice := asSlice(value)
				value = slice[len(slice)-1]
			}
			sub.data = value.(map[string]interface{})
		}

		result = append(result, sub)
		parent = &sub.message
	}
	return result, nil
}

// parseFieldValue parses a field value according to the field type.
// Types: https://developers.google.com/protocol-buffers/docs/proto?hl=en#scalar
func (p *parser) parseFieldValue(s string, msgName string, field *descriptor.FieldDescriptorProto) (interface{}, error) {
	switch field.GetType() {

	case descriptor.FieldDescriptorProto_TYPE_DOUBLE:
		return strconv.ParseFloat(s, 64)

	case descriptor.FieldDescriptorProto_TYPE_FLOAT:
		x, err := strconv.ParseFloat(s, 32)
		return float32(x), err

	case
		descriptor.FieldDescriptorProto_TYPE_INT32,
		descriptor.FieldDescriptorProto_TYPE_SFIXED32,
		descriptor.FieldDescriptorProto_TYPE_SINT32:

		x, err := strconv.ParseInt(s, 10, 32)
		return int32(x), err

	case descriptor.FieldDescriptorProto_TYPE_INT64,
		descriptor.FieldDescriptorProto_TYPE_SFIXED64,
		descriptor.FieldDescriptorProto_TYPE_SINT64:

		return strconv.ParseInt(s, 10, 64)

	case descriptor.FieldDescriptorProto_TYPE_UINT32, descriptor.FieldDescriptorProto_TYPE_FIXED32:
		x, err := strconv.ParseUint(s, 10, 32)
		return uint32(x), err

	case descriptor.FieldDescriptorProto_TYPE_UINT64, descriptor.FieldDescriptorProto_TYPE_FIXED64:
		return strconv.ParseUint(s, 10, 64)

	case descriptor.FieldDescriptorProto_TYPE_BOOL:
		return strconv.ParseBool(s)

	case descriptor.FieldDescriptorProto_TYPE_STRING:
		return s, nil

	case descriptor.FieldDescriptorProto_TYPE_MESSAGE:
		return nil, fmt.Errorf(
			"%s.%s is a message field. Specify its field values, not the message itself",
			msgName, field.GetName())

	case descriptor.FieldDescriptorProto_TYPE_BYTES:
		return hex.DecodeString(s)

	case descriptor.FieldDescriptorProto_TYPE_ENUM:
		obj, err := p.resolve(field.GetTypeName())
		if err != nil {
			return nil, err
		}
		enum, ok := obj.(*descriptor.EnumDescriptorProto)
		if !ok {
			return nil, fmt.Errorf(
				"field %s.%s is declared as of type enum %s, but %s is not an enum",
				msgName, field.GetName(),
				field.GetTypeName(), field.GetTypeName(),
			)
		}
		return parseEnum(enum, s)

	default:
		return nil, fmt.Errorf("field type %s is not supported", field.GetType())
	}
}

func (p *parser) resolve(name string) (interface{}, error) {
	if p.Resolver == nil {
		panic(fmt.Errorf("cannot resolve type %q. Resolver is not set", name))
	}
	name = strings.TrimPrefix(name, ".")
	obj := p.Resolver.Resolve(name)
	if obj == nil {
		return nil, fmt.Errorf("cannot resolve type %q", name)
	}
	return obj, nil
}

// splitKeyValuePair splits a key value pair key=value if there is equals sign.
func (p *parser) splitKeyValuePair(s string) (key, value string, hasValue bool) {
	parts := strings.SplitN(s, "=", 2)
	switch len(parts) {
	case 1:
		key = s
	case 2:
		key = parts[0]
		value = parts[1]
		hasValue = true
	}
	return
}

// parseEnum returns the number of an enum member, which can be name or number.
func parseEnum(enum *descriptor.EnumDescriptorProto, member string) (int32, error) {
	i := enum.FindValue(member)
	if i < 0 {
		// Is member the number?
		if number, err := strconv.ParseInt(member, 10, 32); err == nil {
			i = enum.FindValueByNumber(int32(number))
		}
	}
	if i < 0 {
		return 0, fmt.Errorf("invalid value %q for enum %s", member, enum.GetName())
	}
	return enum.Value[i].GetNumber(), nil
}

func asSlice(x interface{}) []interface{} {
	if x == nil {
		return nil
	}
	return x.([]interface{})
}
