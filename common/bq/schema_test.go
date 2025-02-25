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
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAddMissingFields(t *testing.T) {
	ftt.Run("Rename field", t, func(t *ftt.Test) {
		from := bigquery.Schema{
			{
				Name: "a",
				Type: bigquery.IntegerFieldType,
			},
			{
				Name: "b",
				Type: bigquery.RecordFieldType,
				Schema: bigquery.Schema{
					{
						Name: "c",
						Type: bigquery.IntegerFieldType,
					},
					{
						Name: "d",
						Type: bigquery.IntegerFieldType,
					},
				},
			},
		}
		to := bigquery.Schema{
			{
				Name: "e",
				Type: bigquery.IntegerFieldType,
			},
			{
				Name: "b",
				Type: bigquery.RecordFieldType,
				Schema: bigquery.Schema{
					{
						Name: "c",
						Type: bigquery.IntegerFieldType,
					},
				},
			},
		}
		AddMissingFields(&to, from)
		assert.Loosely(t, to, should.Match(bigquery.Schema{
			{
				Name: "e",
				Type: bigquery.IntegerFieldType,
			},
			{
				Name: "b",
				Type: bigquery.RecordFieldType,
				Schema: bigquery.Schema{
					{
						Name: "c",
						Type: bigquery.IntegerFieldType,
					},
					{
						Name: "d",
						Type: bigquery.IntegerFieldType,
					},
				},
			},
			{
				Name: "a",
				Type: bigquery.IntegerFieldType,
			},
		}))
	})
}

func TestDescription(t *testing.T) {
	ftt.Run("Happy path", t, func(t *ftt.Test) {
		f := &descriptorpb.FileDescriptorProto{}
		int64t := descriptorpb.FieldDescriptorProto_TYPE_INT64
		field := &descriptorpb.FieldDescriptorProto{
			Type: &int64t,
		}
		lComment := " foo is the count of possible microstates in a thermodynamic equilibrium"
		c := &SchemaConverter{
			SourceCodeInfo: map[*descriptorpb.FileDescriptorProto]SourceCodeInfoMap{
				f: map[any]*descriptorpb.SourceCodeInfo_Location{
					field: {
						LeadingComments: &lComment,
					},
				},
			},
		}
		resp := c.description(f, field)
		assert.Loosely(t, strings.TrimSpace(lComment), should.Equal(resp))
	})

	ftt.Run("Happy path with Enum", t, func(t *ftt.Test) {
		pkgName := "bar"
		typeName := "foo"
		fullName := pkgName + "." + typeName
		longName := "fooooooooooooooooooooooooooooooo"
		lComment := " foo is the count of possible microstates in a thermodynamic equilibrium"
		validStates := "\nValid values: "
		f := &descriptorpb.FileDescriptorProto{
			Package: &pkgName,
			EnumType: []*descriptorpb.EnumDescriptorProto{
				{
					Name: &typeName,
					Value: []*descriptorpb.EnumValueDescriptorProto{
						{
							Name: &longName,
						},
					},
				},
			},
		}
		enumt := descriptorpb.FieldDescriptorProto_TYPE_ENUM
		field := &descriptorpb.FieldDescriptorProto{
			Type:     &enumt,
			TypeName: &fullName,
		}
		c := &SchemaConverter{
			Desc: &descriptorpb.FileDescriptorSet{
				File: []*descriptorpb.FileDescriptorProto{
					f,
				},
			},
			SourceCodeInfo: map[*descriptorpb.FileDescriptorProto]SourceCodeInfoMap{
				f: map[any]*descriptorpb.SourceCodeInfo_Location{
					field: {
						LeadingComments: &lComment,
					},
				},
			},
		}
		resp := c.description(f, field)
		expect := lComment + validStates + longName + "."
		assert.Loosely(t, strings.TrimSpace(expect), should.Equal(resp))
	})

	ftt.Run("Happy path with Enum truncated", t, func(t *ftt.Test) {
		pkgName := "bar"
		typeName := "foo"
		fullName := pkgName + "." + typeName
		longNames := make([]*descriptorpb.EnumValueDescriptorProto, 32)
		for i := range 32 {
			name := fmt.Sprintf("fooooooooooooooooooooooooooooooo%d", i)
			longNames[i] = &descriptorpb.EnumValueDescriptorProto{
				Name: &name,
			}
		}
		lComment := " foo is the count of possible microstates in a thermodynamic equilibrium"
		f := &descriptorpb.FileDescriptorProto{
			Package: &pkgName,
			EnumType: []*descriptorpb.EnumDescriptorProto{
				{
					Name:  &typeName,
					Value: longNames,
				},
			},
		}
		enumt := descriptorpb.FieldDescriptorProto_TYPE_ENUM
		field := &descriptorpb.FieldDescriptorProto{
			Type:     &enumt,
			TypeName: &fullName,
		}
		c := &SchemaConverter{
			Desc: &descriptorpb.FileDescriptorSet{
				File: []*descriptorpb.FileDescriptorProto{
					f,
				},
			},
			SourceCodeInfo: map[*descriptorpb.FileDescriptorProto]SourceCodeInfoMap{
				f: map[any]*descriptorpb.SourceCodeInfo_Location{
					field: {
						LeadingComments: &lComment,
					},
				},
			},
		}
		resp := c.description(f, field)
		expect := lComment + "...(truncated)"
		assert.Loosely(t, strings.TrimSpace(expect), should.Equal(resp))
	})
}
