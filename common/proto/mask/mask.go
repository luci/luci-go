// Copyright 2020 The LUCI Authors.
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

//Package mask provides utility functions for google protobuf field mask
//
// Supports advanced field mask semantics:
// - Refer to fields and map keys using . literals:
//   - Supported map key types: string, integer, bool. (double, float, enum,
//     and bytes keys are not supported by protobuf or this implementation)
//   - Fields: "publisher.name" means field "name" of field "publisher"
//   - String map keys: "metadata.year" means string key 'year' of map field
//     metadata
//   - Integer map keys (e.g. int32): 'year_ratings.0' means integer key 0 of
//     a map field year_ratings
//   - Bool map keys: 'access_text.true' means boolean key true of a map field
//     access_text
// - String map keys that cannot be represented as an unquoted string literal,
//   must be quoted using backticks: metadata.`year.published`, metadata.`17`,
//   metadata.``. Backtick can be escaped with ``: a.`b``c` means map key "b`c"
//   of map field a.
// - Refer to all map keys using a * literal: "topics.*.archived" means field
//   "archived" of all map values of map field "topic".
// - Refer to all elements of a repeated field using a * literal: authors.*.name
// - Refer to all fields of a message using * literal: publisher.*.
// - Prohibit addressing a single element in repeated fields: authors.0.name
//
// FieldMask.paths string grammar:
//   path = segment {'.' segment}
//   segment = literal | star | quoted_string;
//   literal = string | integer | boolean
//   string = (letter | '_') {letter | '_' | digit}
//   integer = ['-'] digit {digit};
//   boolean = 'true' | 'false';
//   quoted_string = '`' { utf8-no-backtick | '``' } '`'
//   star = '*'
package mask

import (
	"fmt"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoimpl"
)

// mask is a tree representation of a field mask. Serves as a tree node too.
// Each node represents a segment of a path, e.g. 'bar' in 'foo.bar.qux'.
// A Field mask with paths ['a', 'b.c'] is parsed as
// <root>
//   a
//   b
//    c
type mask struct {
	descriptor protoreflect.MessageDescriptor
	isRepeated bool
	children   map[string]mask
}

// FromFieldMask parses a field mask to a mask.
//
// Trailing stars will be removed, e.g. parses ['a.*'] as ['a'].
// Redundant paths will be removed, e.g. parses ['a', 'a.b'] as ['a'].
//
// If isFieldNameJSON is set to true, json name will be used instead of
// canonical name defined in proto during parsing (e.g. "fooBar" instead of
// "foo_bar"). However, the child field name in return mask will always be
// in canonical form.
//
// If isUpdateMask is set to true, a repeated field is allowed only as the last
// field in a path string.
func FromFieldMask(fieldMask *field_mask.FieldMask, targeMsg proto.Message, isFieldNameJSON bool, isUpdateMask bool) (mask, error) {
	descriptor := protoimpl.X.MessageDescriptorOf(targeMsg)
	parsedPaths := make([]path, len(fieldMask.GetPaths()))
	for i, p := range fieldMask.GetPaths() {
		parsedPath, err := parsePath(p, descriptor, isFieldNameJSON)
		if err != nil {
			return mask{}, err
		}
		parsedPaths[i] = parsedPath
	}
	return fromParsedPaths(parsedPaths, descriptor, isUpdateMask)
}

// fromParsedPaths constructs a mask tree from a slice of parsed paths.
func fromParsedPaths(parsedPaths []path, desc protoreflect.MessageDescriptor, isUpdateMask bool) (mask, error) {
	root := mask{
		descriptor: desc,
		children:   map[string]mask{},
	}
	for _, p := range normalizedPaths(parsedPaths) {
		curNode := root
		curNodeName := ""
		for _, seg := range p {
			if curNode.isRepeated && isUpdateMask {
				return mask{}, fmt.Errorf("update mask allows a repeated field only at the last position; field: %s is not last", curNodeName)
			}
			if _, found := curNode.children[seg]; !found {
				var child mask
				switch curDesc := curNode.descriptor; {
				case curDesc.IsMapEntry():
					child = mask{
						descriptor: curDesc.Fields().ByName(protoreflect.Name("value")).Message(),
						children:   map[string]mask{},
					}
					break
				case curNode.isRepeated:
					child = mask{
						descriptor: curDesc,
						children:   map[string]mask{},
					}
					break
				default:
					field := curDesc.Fields().ByName(protoreflect.Name(seg))
					child = mask{
						descriptor: field.Message(),
						isRepeated: field.Cardinality() == protoreflect.Repeated,
						children:   map[string]mask{},
					}
				}
				curNode.children[seg] = child
			}
			curNode = curNode.children[seg]
			curNodeName = seg
		}
	}
	return root, nil
}

type byDepth []path

func (paths byDepth) Len() int   { return len(paths) }
func (ps byDepth) Swap(i, j int) { ps[i], ps[j] = ps[j], ps[i] }
func (ps byDepth) Less(i, j int) bool {
	lenI, lenJ := len(ps[i]), len(ps[j])
	if lenI == lenJ {
		for index, segI := range ps[i] {
			if segI == ps[j][index] {
				continue
			}
			return segI < ps[j][index]
		}
		return true
	}
	return lenI < lenJ
}

// normalizedPaths normalizes parsed paths. Returns a new slice of paths
//
// Removes paths that have a segment prefix already present in paths,
// e.g. removes ["a", "b"] from [["a", "b"], ["a",]].
//
// The result slice is stable and ordered by the number of segments of each
// path. If two paths have same number of segments, break the tie by comparing
// the segments at each index alphabetically.
func normalizedPaths(paths []path) []path {
	present := map[string]bool{}
	delimiter := string(pathDelimiter)
	ret := []path{}
	sort.SliceStable(paths, func(i, j int) bool {
		lenI, lenJ := len(paths[i]), len(paths[j])
		if lenI == lenJ {
			for index, segI := range paths[i] {
				if segI == paths[j][index] {
					continue
				}
				return segI < paths[j][index]
			}
			return true
		}
		return lenI < lenJ
	})

PATH_LOOP:
	for _, p := range paths {
		for i := range p {
			if present[strings.Join(p[:i+1], delimiter)] {
				continue PATH_LOOP
			}
		}
		ret = append(ret, p)
		present[strings.Join(p, delimiter)] = true
	}
	return ret
}

func (m mask) Trim(msg proto.Message) proto.Message {
	panic("not implemented")
}

type Inclusiveness int8

const (
	Exclude Inclusiveness = iota
	PartiallyInclude
	EntirelyInclude
)

func (m mask) Includes(path string) (Inclusiveness, error) {
	panic("not implemented")
}

func (m mask) Merge(src proto.Message, dest proto.Message) error {
	panic("not implemented")
}

func (m mask) Submask(path string) (mask, bool, error) {
	panic("not implemented")
}
