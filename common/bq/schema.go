// Copyright 2018 The LUCI Authors.
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

package bq

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"

	"github.com/pmezard/go-difflib/difflib"

	bqpb "go.chromium.org/luci/common/bq/pb"
	"go.chromium.org/luci/common/data/text/indented"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google/descutil"
)

const truncMsg = "...(truncated)"

// The maximum length of a BigQuery field description is 1024 characters.
// https://cloud.google.com/bigquery/quotas#all_tables
const fieldDescriptionMaxlength = 1024 - len(truncMsg)

// SourceCodeInfoMap maps descriptor proto messages to source code info,
// if available.
// See also descutil.IndexSourceCodeInfo.
type SourceCodeInfoMap map[any]*descriptorpb.SourceCodeInfo_Location

type SchemaConverter struct {
	Desc           *descriptorpb.FileDescriptorSet
	SourceCodeInfo map[*descriptorpb.FileDescriptorProto]SourceCodeInfoMap
}

// Schema constructs a bigquery.Schema from a named message.
func (c *SchemaConverter) Schema(messageName string) (schema bigquery.Schema, description string, err error) {
	file, obj, _ := descutil.Resolve(c.Desc, messageName)
	if obj == nil {
		return nil, "", fmt.Errorf("message %q is not found", messageName)
	}
	msg, isMsg := obj.(*descriptorpb.DescriptorProto)
	if !isMsg {
		return nil, "", fmt.Errorf("expected %q to be a message, but it is %T", messageName, obj)
	}

	schema = make(bigquery.Schema, 0, len(msg.Field))
	for _, field := range msg.Field {
		switch s, err := c.field(file, field); {
		case err != nil:
			return nil, "", errors.Annotate(err, "failed to derive schema for field %q in message %q", field.GetName(), msg.GetName()).Err()
		case s != nil:
			schema = append(schema, s)
		}
	}
	return schema, c.description(file, msg), nil
}

// field constructs bigquery.FieldSchema from proto field descriptor.
func (c *SchemaConverter) field(file *descriptorpb.FileDescriptorProto, field *descriptorpb.FieldDescriptorProto) (*bigquery.FieldSchema, error) {
	schema := &bigquery.FieldSchema{
		Name:        field.GetName(),
		Description: c.description(file, field),
		Repeated:    descutil.Repeated(field),
		Required:    descutil.Required(field),
	}

	// If have bqschema.options field option, just grab the type from there.
	if field.Options != nil && proto.HasExtension(field.Options, bqpb.E_Options) {
		meta := proto.GetExtension(field.Options, bqpb.E_Options).(*bqpb.FieldOptions)
		if typ := meta.GetBqType(); typ != "" {
			schema.Type = bigquery.FieldType(typ)
			return schema, nil
		}
	}

	typeName := strings.TrimPrefix(field.GetTypeName(), ".")
	switch field.GetType() {
	case
		descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:

		schema.Type = bigquery.FloatFieldType

	case
		descriptorpb.FieldDescriptorProto_TYPE_INT64,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_INT32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_SFIXED64,
		descriptorpb.FieldDescriptorProto_TYPE_SINT32,
		descriptorpb.FieldDescriptorProto_TYPE_SINT64:

		schema.Type = bigquery.IntegerFieldType

	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		schema.Type = bigquery.BooleanFieldType

	case descriptorpb.FieldDescriptorProto_TYPE_STRING:
		schema.Type = bigquery.StringFieldType

	case descriptorpb.FieldDescriptorProto_TYPE_BYTES:
		schema.Type = bigquery.BytesFieldType

	case descriptorpb.FieldDescriptorProto_TYPE_ENUM:
		schema.Type = bigquery.StringFieldType

	case descriptorpb.FieldDescriptorProto_TYPE_MESSAGE:
		switch typeName {
		case "google.protobuf.Duration":
			schema.Type = bigquery.FloatFieldType
		case "google.protobuf.Timestamp":
			schema.Type = bigquery.TimestampFieldType
		case "google.protobuf.Struct":
			// google.protobuf.Struct is persisted as JSONPB string.
			// See also https://bit.ly/chromium-bq-struct
			schema.Type = bigquery.StringFieldType
		default:
			switch s, _, err := c.Schema(typeName); {
			case err != nil:
				return nil, err
			case len(s) == 0:
				// BigQuery does not like empty record fields.
				return nil, nil
			default:
				schema.Type = bigquery.RecordFieldType
				schema.Schema = s
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
// "\nValid values: <comma-separated enum member names>" if it fits within the limit.
func (c *SchemaConverter) description(file *descriptorpb.FileDescriptorProto, ptr any) string {
	description := c.SourceCodeInfo[file][ptr].GetLeadingComments()

	// Append valid enum values.
	if field, ok := ptr.(*descriptorpb.FieldDescriptorProto); ok && field.GetType() == descriptorpb.FieldDescriptorProto_TYPE_ENUM {
		_, obj, _ := descutil.Resolve(c.Desc, strings.TrimPrefix(field.GetTypeName(), "."))
		if enum, ok := obj.(*descriptorpb.EnumDescriptorProto); ok {
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

	totalSize := 0
	end := len(lines)
	for i := range lines {
		if trimSize > 0 && len(lines[i]) >= trimSize {
			lines[i] = lines[i][trimSize:]
		}
		lineLen := len(lines[i]) + 1 // add 1 for the ending "\n".
		if totalSize+lineLen > fieldDescriptionMaxlength {
			// description is too long, discard the rest of it.
			end = i
			break
		}
		totalSize += lineLen
	}
	description = strings.Join(lines[0:end], "\n")
	description = strings.TrimSpace(description)
	if end < len(lines) {
		description += truncMsg
	}

	return description
}

func printSchema(w *indented.Writer, s bigquery.Schema) {
	// Field order does not matter.
	// A new field is always added to the end of the field list in a live table.
	// Sort fields by name to make the result deterministic.
	// Schema diffing relies on it.

	s = append(bigquery.Schema(nil), s...)
	sort.Slice(s, func(i, j int) bool {
		return s[i].Name < s[j].Name
	})

	for i, f := range s {
		if i > 0 {
			fmt.Fprintln(w)
		}

		if f.Description != "" {
			for _, line := range strings.Split(f.Description, "\n") {
				fmt.Fprintln(w, "//", line)
			}
		}

		switch {
		case f.Repeated:
			fmt.Fprint(w, "repeated ")
		case f.Required:
			fmt.Fprint(w, "required ")
		}

		fmt.Fprintf(w, "%s %s", f.Type, f.Name)

		if f.Type == bigquery.RecordFieldType {
			fmt.Fprintln(w, " {")
			w.Level++
			printSchema(w, f.Schema)
			w.Level--
			fmt.Fprint(w, "}")
		}

		fmt.Fprintln(w)
	}
}

// SchemaString returns schema in string format.
func SchemaString(s bigquery.Schema) string {
	var buf bytes.Buffer
	printSchema(&indented.Writer{Writer: &buf}, s)
	return buf.String()
}

// SchemaDiff returns unified diff of two schemas.
// Returns "" if there is no difference.
func SchemaDiff(before, after bigquery.Schema) string {
	ret, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(SchemaString(before)),
		B:        difflib.SplitLines(SchemaString(after)),
		FromFile: "Current",
		ToFile:   "New",
		Context:  3,
		Eol:      "\n",
	})
	if err != nil {
		// GetUnifiedDiffString returns an error only if it fails
		// to write to a bytes.Buffer, which either cannot happen or we better
		// panic.
		panic(err)
	}
	return ret
}

// AddMissingFields copies fields from src to dest if they are not present in
// dest.
func AddMissingFields(dest *bigquery.Schema, src bigquery.Schema) {
	destFields := indexFields(*dest)
	for _, sf := range src {
		switch df := destFields[sf.Name]; {
		case df == nil:
			*dest = append(*dest, sf)
		default:
			AddMissingFields(&df.Schema, sf.Schema)
		}
	}
}

func indexFields(s bigquery.Schema) map[string]*bigquery.FieldSchema {
	ret := make(map[string]*bigquery.FieldSchema, len(s))
	for _, f := range s {
		ret[f.Name] = f
	}
	return ret
}
