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

// Package mask provides utility functions for google protobuf field mask
//
// Supports advanced field mask semantics:
//  - Refer to fields and map keys using . literals:
//    - Supported map key types: string, integer, bool. (double, float, enum,
//     and bytes keys are not supported by protobuf or this implementation)
//    - Fields: "publisher.name" means field "name" of field "publisher"
//    - String map keys: "metadata.year" means string key 'year' of map field
//     metadata
//    - Integer map keys (e.g. int32): 'year_ratings.0' means integer key 0 of
//     a map field year_ratings
//    - Bool map keys: 'access_text.true' means boolean key true of a map field
//     access_text
//  - String map keys that cannot be represented as an unquoted string literal,
//   must be quoted using backticks: metadata.`year.published`, metadata.`17`,
//   metadata.``. Backtick can be escaped with ``: a.`b``c` means map key "b`c"
//   of map field a.
//  - Refer to all map keys using a * literal: "topics.*.archived" means field
//   "archived" of all map values of map field "topic".
//  - Refer to all elements of a repeated field using a * literal: authors.*.name
//  - Refer to all fields of a message using * literal: publisher.*.
//  - Prohibit addressing a single element in repeated fields: authors.0.name
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

	// TODO(yiwzhang): Get rid of protoimpl import by calling proto.MessageReflect
	// and proto.MessageV1 instead once we've successfully upgrade the version of
	// go protobuf library from v1.3 to v1.4 as these functions only exist in
	// latest version. It is discouraged to import protoimpl as per its pkg doc.
	"google.golang.org/protobuf/runtime/protoimpl"

	"go.chromium.org/luci/common/data/stringset"
)

// Mask is a tree representation of a field Mask. Serves as a tree node too.
// Each node represents a segment of a path, e.g. "bar" in "foo.bar.qux".
// A Field Mask with paths ["a","b.c"] is parsed as
//    <root>
//    /    \
//  "a"    "b"
//         /
//       "c"
type Mask struct {
	// descriptor is the proto descriptor of the message of the field this node
	// represents. If the field kind is not a message, then descriptor is nil and
	// the node must be a leaf unless isRepeated is true which denotes a repeated
	// scalar field.
	descriptor protoreflect.MessageDescriptor
	// isRepeated indicates whether the segment represents a repeated field or
	// not. Children of this node are the field elements.
	isRepeated bool
	// children maps segments to its node. e.g. children of the root in the
	// example above has keys "a" and "b", and values are Mask objects and the
	// Mask object "b" maps to will have a single child "c". All types of segment
	// (i.e. int, bool, string, star) will be converted to string.
	children map[string]Mask
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
func FromFieldMask(fieldMask *field_mask.FieldMask, targeMsg proto.Message, isFieldNameJSON bool, isUpdateMask bool) (Mask, error) {
	descriptor := protoimpl.X.MessageDescriptorOf(targeMsg)
	parsedPaths := make([]path, len(fieldMask.GetPaths()))
	for i, p := range fieldMask.GetPaths() {
		parsedPath, err := parsePath(p, descriptor, isFieldNameJSON)
		if err != nil {
			return Mask{}, err
		}
		parsedPaths[i] = parsedPath
	}
	return fromParsedPaths(parsedPaths, descriptor, isUpdateMask)
}

// fromParsedPaths constructs a mask tree from a slice of parsed paths.
func fromParsedPaths(parsedPaths []path, desc protoreflect.MessageDescriptor, isUpdateMask bool) (Mask, error) {
	root := Mask{
		descriptor: desc,
		children:   map[string]Mask{},
	}
	for _, p := range normalizePaths(parsedPaths) {
		curNode := root
		curNodeName := ""
		for _, seg := range p {
			if curNode.isRepeated && isUpdateMask {
				return Mask{}, fmt.Errorf("update mask allows a repeated field only at the last position; field: %s is not last", curNodeName)
			}
			if _, ok := curNode.children[seg]; !ok {
				child := Mask{
					children: map[string]Mask{},
				}
				switch curDesc := curNode.descriptor; {
				case curDesc.IsMapEntry():
					child.descriptor = curDesc.Fields().ByName(protoreflect.Name("value")).Message()
				case curNode.isRepeated:
					child.descriptor = curDesc
				default:
					field := curDesc.Fields().ByName(protoreflect.Name(seg))
					child.descriptor = field.Message()
					child.isRepeated = field.Cardinality() == protoreflect.Repeated
				}
				curNode.children[seg] = child
			}
			curNode = curNode.children[seg]
			curNodeName = seg
		}
	}
	return root, nil
}

// normalizePaths normalizes parsed paths. Returns a new slice of paths.
//
// Removes paths that have a segment prefix already present in paths,
// e.g. removes ["a", "b"] from [["a", "b"], ["a",]].
//
// The result slice is stable and ordered by the number of segments of each
// path. If two paths have same number of segments, break the tie by comparing
// the segments at each index lexicographically.
func normalizePaths(paths []path) []path {
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

	present := stringset.New(len(paths))
	delimiter := string(pathDelimiter)
	ret := make([]path, 0, len(paths))
PATH_LOOP:
	for _, p := range paths {
		for i := range p {
			if present.Has(strings.Join(p[:i+1], delimiter)) {
				continue PATH_LOOP
			}
		}
		ret = append(ret, p)
		present.Add(strings.Join(p, delimiter))
	}
	return ret
}

// Trim clears protobuf message fields that are not in the mask.
//
// If mask is empty, this is a noop. It returns error when the supplied
// message is nil or has a different message descriptor from that of mask.
// It uses Includes to decide what to trim, see its doc.
func (m Mask) Trim(msg proto.Message) error {
	if m.IsEmpty() {
		return nil
	}
	reflectMsg := protoimpl.X.MessageOf(msg)
	if err := checkMsgHaveDesc(reflectMsg, m.descriptor); err != nil {
		return err
	}
	m.trimImpl(reflectMsg)
	return nil
}

func (m Mask) trimImpl(reflectMsg protoreflect.Message) {
	reflectMsg.Range(func(fieldDesc protoreflect.FieldDescriptor, fieldVal protoreflect.Value) bool {
		fieldName := string(fieldDesc.Name())
		switch incl, _ := m.includesImpl(path{fieldName}); incl {
		case Exclude:
			reflectMsg.Clear(fieldDesc)
		case IncludePartially:
			// child for this field must exist because the path is included partially
			switch child := m.children[fieldName]; {
			case fieldDesc.IsMap():
				child.trimMap(fieldVal.Map(), fieldDesc.MapValue().Kind())
			case fieldDesc.Kind() != protoreflect.MessageKind:
				// The field is scalar but the mask does not specify to include
				// it entirely. Skip it because scalars do not have subfields.
				// Note that FromFieldMask would fail on such a mask because a
				// scalar field cannot be followed by other fields.
				reflectMsg.Clear(fieldDesc)
			case fieldDesc.IsList():
				// star child is the only possible child for list field
				if starChild, ok := child.children["*"]; ok {
					for i, list := 0, fieldVal.List(); i < list.Len(); i++ {
						starChild.trimImpl(list.Get(i).Message())
					}
				}
			default:
				child.trimImpl(fieldVal.Message())
			}
		}
		return true
	})
}

func (m Mask) trimMap(protoMap protoreflect.Map, valueKind protoreflect.Kind) {
	protoMap.Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
		keyString := k.String()
		switch incl, _ := m.includesImpl(path{keyString}); {
		case incl == Exclude:
			protoMap.Clear(k)
		case incl == IncludePartially && valueKind != protoreflect.MessageKind:
			// same reason as comment above that value is scalar
			protoMap.Clear(k)
		case incl == IncludePartially:
			// Mask might not have a child of keyName but it can still partially
			// include the key because of star child. So, check both key child
			// and star child.
			for _, seg := range []string{keyString, "*"} {
				if child, ok := m.children[seg]; ok {
					child.trimImpl(v.Message())
				}
			}
		}
		return true
	})
}

// Inclusiveness tells if a field value at the given path is included.
type Inclusiveness int8

const (
	// Exclude indicates the field value is excluded.
	Exclude Inclusiveness = iota
	// IncludePartially indicates some subfields of the field value are included.
	IncludePartially
	// IncludeEntirely indicates the entire field value is included.
	IncludeEntirely
)

// Includes tells the Inclusiveness of a field value at the given path.
//
// The path must have canonical field names, i.e. not JSON names.
// Returns error if path parsing fails.
func (m Mask) Includes(path string) (Inclusiveness, error) {
	parsedPath, err := parsePath(path, m.descriptor, false)
	if err != nil {
		return Exclude, err
	}
	incl, _ := m.includesImpl(parsedPath)
	return incl, nil
}

// includesImpl implements Includes(). It returns the computed inclusiveness
// and the leaf mask that includes the path if IncludeEntirely, or the
// intermediate mask that the last segment of path represents if
// IncludePartially or an empty mask if Exclude.
func (m Mask) includesImpl(p path) (Inclusiveness, Mask) {
	if len(m.children) == 0 {
		return IncludeEntirely, m
	}
	if len(p) == 0 {
		// This node is intermediate and we've exhausted the path. Some of the
		// value's subfields are included, so includes this value partially.
		return IncludePartially, m
	}

	incl, inclMask := Exclude, Mask{}
	// star child should also be examined.
	// e.g. children are {"a": {"b": {}}, "*": {"c": {}}}
	// If seg is 'x', we should check the star child.
	for _, seg := range []string{p[0], "*"} {
		if child, ok := m.children[seg]; ok {
			if cIncl, cInclMask := child.includesImpl(p[1:]); cIncl > incl {
				incl, inclMask = cIncl, cInclMask
			}
		}
	}
	return incl, inclMask
}

// Merge merges masked fields from src to dest.
//
// Empty field will be merged as long as they are present in the mask. Repeated
// fields or map fields will be overwritten entirely. Partial updates are not
// supported for such field.
func (m Mask) Merge(src proto.Message, dest proto.Message) error {
	panic("not implemented")
}

// Submask returns a sub-mask given a path from the received mask to it.
//
// For example, for a mask ["a.b.c"], m.submask("a.b") will return a mask ["c"].
//
// If the received mask includes the path entirely, returns a Mask that includes
// everything. For example, for mask ["a"], m.submask("a.b") returns a mask
// without children.
//
// Returns error if path parsing fails or path is excluded from the received
// mask
func (m Mask) Submask(path string) (Mask, error) {
	panic("not implemented")
}

// IsEmpty reports whether a mask is of empty value. Such mask implies keeping
// everything when calling Trim, merging nothing when calling Merge and always
// returning IncludeEntirely when calling Includes
func (m Mask) IsEmpty() bool {
	return m.descriptor == nil && !m.isRepeated && len(m.children) == 0
}

// checkMsgHaveDesc validates that the descriptor of given proto reflect message
// matches the expected message descriptor. It returns error when the given
// message is nil or descriptor of which doesn't match the expectation.
func checkMsgHaveDesc(msg protoreflect.Message, expectedDesc protoreflect.MessageDescriptor) error {
	if msg == nil {
		return fmt.Errorf("nil message")
	}
	if msgDesc := msg.Descriptor(); msgDesc != expectedDesc {
		return fmt.Errorf("expected message have descriptor: %s; got descriptor: %s", expectedDesc.FullName(), msgDesc.FullName())
	}
	return nil
}
