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

// Package mask provides utility functions for google protobuf field masks.
package mask

import (
	"fmt"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/data/stringset"
)

// Mask is a tree representation of a field Mask. Serves as a tree node too.
// Each node represents a segment of a path, e.g. "bar" in "foo.bar.qux".
// A Field Mask with paths ["a","b.c"] is parsed as
//
//	  <root>
//	  /    \
//	"a"    "b"
//	       /
//	     "c"
//
// Zero value is not valid. Use IsEmpty() to check if the mask is zero.
type Mask struct {
	// descriptor is the proto descriptor of the message of the field this node
	// represents. If the field kind is not a message, then descriptor is nil and
	// the node must be a leaf unless isRepeated is true which denotes a repeated
	// scalar field.
	descriptor protoreflect.MessageDescriptor
	// isRepeated indicates whether the segment represents a repeated field or
	// not. Children of this node are the field elements.
	isRepeated bool
	// isAdvanced is true if this mask was parsed with AdvancedSemantics option.
	isAdvanced bool
	// children maps segments to its node. e.g. children of the root in the
	// example above has keys "a" and "b", and values are Mask objects and the
	// Mask object "b" maps to will have a single child "c". All types of segment
	// (i.e. int, bool, string, star) will be converted to string.
	children map[string]*Mask
}

// Option is implemented by options that can be passed to FromFieldMask.
type Option interface{ apply(*optionSet) }

type optionSet struct {
	advancedSemantics bool // TODO: currently ignored
	isUpdateMask      bool
}

type advancedSemanticsOpt struct{}
type forUpdateOpt struct{}

func (advancedSemanticsOpt) apply(s *optionSet) { s.advancedSemantics = true }
func (forUpdateOpt) apply(s *optionSet)         { s.isUpdateMask = true }

// AdvancedSemantics enables non-standard field mask semantics.
//
// WARNING: advanced field masks never became standard and FieldMask messages
// that contain `*` or other non-standard syntax are not supported by Go's
// jsonpb decoder (it fails to unmarshal them). Using them requires hacks, such
// as go.chromium.org/luci/common/proto.UnmarshalJSONWithNonStandardFieldMasks.
//
// Advanced field mask semantics:
//   - Refer to fields and map keys using . literals:
//   - Supported map key types: string, integer, bool. (double, float, enum,
//     and bytes keys are not supported by protobuf or this implementation)
//   - Fields: "publisher.name" means field "name" of field "publisher"
//   - String map keys: "metadata.year" means string key 'year' of map field
//     metadata
//   - Integer map keys (e.g. int32): 'year_ratings.0' means integer key 0 of
//     a map field year_ratings
//   - Bool map keys: 'access_text.true' means boolean key true of a map field
//     access_text
//   - String map keys that cannot be represented as an unquoted string literal,
//     must be quoted using backticks: metadata.`year.published`, metadata.`17`,
//     metadata.“. Backtick can be escaped with “: a.`b“c` means map key "b`c"
//     of map field a.
//   - Refer to all map keys using a * literal: "topics.*.archived" means field
//     "archived" of all map values of map field "topic".
//   - Refer to all elements of a repeated field using a * literal: authors.*.name
//   - Refer to all fields of a message using * literal: publisher.*.
//   - Prohibit addressing a single element in repeated fields: authors.0.name
//
// Advanced FieldMask.paths string grammar:
//
//	path = segment {'.' segment}
//	segment = literal | star | quoted_string;
//	literal = string | integer | boolean
//	string = (letter | '_') {letter | '_' | digit}
//	integer = ['-'] digit {digit};
//	boolean = 'true' | 'false';
//	quoted_string = '`' { utf8-no-backtick | '``' } '`'
//	star = '*'
func AdvancedSemantics() Option {
	return advancedSemanticsOpt{}
}

// ForUpdate indicates this field mask is used for an update operation.
//
// In such field masks a repeated field is allowed only as the last field in a
// path string.
func ForUpdate() Option {
	return forUpdateOpt{}
}

// FromFieldMask parses a field mask, matching it to the given proto message
// descriptor.
//
// targetMsg is only used for its type. Use Trim to actually apply a mask to
// a proto message.
func FromFieldMask(fieldMask *fieldmaskpb.FieldMask, targetMsg proto.Message, opts ...Option) (*Mask, error) {
	var optSet optionSet
	for _, opt := range opts {
		opt.apply(&optSet)
	}
	descriptor := targetMsg.ProtoReflect().Descriptor()
	parsedPaths := make([]path, len(fieldMask.GetPaths()))
	for i, p := range fieldMask.GetPaths() {
		parsedPath, err := parsePath(p, descriptor, optSet.advancedSemantics)
		if err != nil {
			return nil, err
		}
		parsedPaths[i] = parsedPath
	}
	return fromParsedPaths(parsedPaths, descriptor, optSet)
}

// MustFromReadMask is a shortcut FromFieldMask that accepts field mask as
// variadic paths and that panics if the mask is invalid.
//
// It is useful when the mask is hardcoded. Understands advanced semantics.
func MustFromReadMask(targetMsg proto.Message, paths ...string) *Mask {
	ret, err := FromFieldMask(&fieldmaskpb.FieldMask{Paths: paths}, targetMsg, AdvancedSemantics())
	if err != nil {
		panic(err)
	}
	return ret
}

// All returns a field mask that selects all fields.
func All(targetMsg proto.Message) *Mask {
	return MustFromReadMask(targetMsg, "*")
}

// fromParsedPaths constructs a mask tree from a slice of parsed paths.
func fromParsedPaths(parsedPaths []path, desc protoreflect.MessageDescriptor, opts optionSet) (*Mask, error) {
	root := &Mask{
		descriptor: desc,
		isAdvanced: opts.advancedSemantics,
	}
	ensureChildren := func(n *Mask, hint int) map[string]*Mask {
		if n.children == nil {
			n.children = make(map[string]*Mask, hint)
		}
		return n.children
	}
	for _, p := range normalizePaths(parsedPaths) {
		curNode := root
		curNodeName := ""
		for _, seg := range p {
			if err := validateMask(curNode, curNodeName, seg, opts.isUpdateMask); err != nil {
				return nil, err
			}
			if _, ok := curNode.children[seg]; !ok {
				child := &Mask{isAdvanced: opts.advancedSemantics}
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
				ensureChildren(curNode, len(p))[seg] = child
			}
			curNode = curNode.children[seg]
			curNodeName = seg
		}
	}
	return root, nil
}

// normalizePaths normalizes parsed paths. Returns a new slice of paths.
//
// Removes trailing stars for all paths, e.g. converts ["a", "*"] to ["a"].
// Removes paths that have a segment prefix already present in paths,
// e.g. removes ["a", "b"] from [["a", "b"], ["a",]].
//
// The result slice is stable and ordered by the number of segments of each
// path. If two paths have same number of segments, break the tie by comparing
// the segments at each index lexicographically.
func normalizePaths(paths []path) []path {
	paths = removeTrailingStars(paths)
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

func removeTrailingStars(paths []path) []path {
	ret := make([]path, 0, len(paths))
	for _, p := range paths {
		if n := len(p); n > 0 && p[n-1] == "*" {
			p = p[:n-1]
		}
		ret = append(ret, p)
	}
	return ret
}

func validateMask(node *Mask, nodeName string, childName string, isUpdateMask bool) error {
	if node.isRepeated && isUpdateMask {
		nodeDesc := node.descriptor
		// For a map, https://google.aip.dev/161 states that field masks may permit
		// the specification of specific fields in a map, e.g. "my_map.some_key",
		// if and only if the map's keys are either strings or integers.
		// This can be used when a request tries to update a specific key in a map.
		//
		// Note that the requirement for the key type is already guaranteed by
		// protobuf spec: https://protobuf.dev/programming-guides/proto3/#maps,
		if nodeDesc.IsMapEntry() && childName != "*" {
			return nil
		}
		return fmt.Errorf("update mask allows a repeated field only at the last position; field: %s is not last", nodeName)
	}
	return nil
}

// Children returns the children of the current Mask node.
func (m *Mask) Children() map[string]*Mask {
	return m.children
}

// Trim clears protobuf message fields that are not in the mask.
//
// If mask is empty, this is a noop. It returns error when the supplied
// message is nil or has a different message descriptor from that of mask.
// It uses Includes to decide what to trim, see its doc.
func (m *Mask) Trim(msg proto.Message) error {
	if m.IsEmpty() {
		return nil
	}
	reflectMsg, err := checkMsgHaveDesc(msg, m.descriptor)
	if err != nil {
		return err
	}
	m.trimImpl(reflectMsg)
	return nil
}

func (m *Mask) trimImpl(reflectMsg protoreflect.Message) {
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

func (m *Mask) trimMap(protoMap protoreflect.Map, valueKind protoreflect.Kind) {
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
// Returns error if path parsing fails.
func (m *Mask) Includes(path string) (Inclusiveness, error) {
	parsedPath, err := parsePath(path, m.descriptor, m.isAdvanced)
	if err != nil {
		return Exclude, err
	}
	incl, _ := m.includesImpl(parsedPath)
	return incl, nil
}

// MustIncludes tells the Inclusiveness of a field value at the given path.
//
// This is essentially the same as Includes, but panics if the given path is
// invalid.
func (m *Mask) MustIncludes(path string) Inclusiveness {
	incl, err := m.Includes(path)
	if err != nil {
		panic(fmt.Sprintf("MustIncludes(%q): %s", path, err))
	}
	return incl
}

// includesImpl implements Includes(). It returns the computed inclusiveness
// and the leaf mask that includes the path if IncludeEntirely, or the
// intermediate mask that the last segment of path represents if
// IncludePartially or an empty mask if Exclude.
func (m *Mask) includesImpl(p path) (Inclusiveness, *Mask) {
	if len(m.children) == 0 {
		return IncludeEntirely, m
	}
	if len(p) == 0 {
		// This node is intermediate and we've exhausted the path. Some of the
		// value's subfields are included, so includes this value partially.
		return IncludePartially, m
	}

	var incl Inclusiveness
	var inclMask *Mask
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
// If mask is empty, this is a noop. It returns error when one of src or dest
// message is nil or has different message descriptor from that of mask.
// Empty field will be merged as long as they are present in the mask. Repeated
// fields or map fields will be overwritten entirely. Partial updates are not
// supported for such field.
func (m *Mask) Merge(src, dest proto.Message) error {
	if m.IsEmpty() {
		return nil
	}
	srcReflectMsg, err := checkMsgHaveDesc(src, m.descriptor)
	if err != nil {
		return fmt.Errorf("src message: %s", err.Error())
	}
	destReflectMsg, err := checkMsgHaveDesc(dest, m.descriptor)
	if err != nil {
		return fmt.Errorf("dest message: %s", err.Error())
	}
	m.mergeImpl(srcReflectMsg, destReflectMsg)
	return nil
}

func (m *Mask) mergeImpl(src, dest protoreflect.Message) {
	for seg, submask := range m.children {
		// star field is not supported for update mask so this won't be nil
		fieldDesc := m.descriptor.Fields().ByName(protoreflect.Name(seg))
		switch srcVal, kind := src.Get(fieldDesc), fieldDesc.Kind(); {
		case fieldDesc.IsList():
			newField := dest.NewField(fieldDesc)
			srcList, destList := srcVal.List(), newField.List()
			for i := range srcList.Len() {
				destList.Append(cloneValue(srcList.Get(i), kind))
			}
			dest.Set(fieldDesc, newField)

		case fieldDesc.IsMap():
			newField := dest.NewField(fieldDesc)
			destMap := newField.Map()
			mapValKind := fieldDesc.MapValue().Kind()
			srcVal.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
				destMap.Set(k, cloneValue(v, mapValKind))
				return true
			})
			dest.Set(fieldDesc, newField)

		case fieldDesc.Kind() == protoreflect.MessageKind:
			switch srcNil := !srcVal.Message().IsValid(); {
			case srcNil && !dest.Get(fieldDesc).Message().IsValid():
				// dest is also nil message. No need to proceed merging.
			case srcNil && len(submask.children) == 0:
				dest.Clear(fieldDesc)
			case len(submask.children) == 0:
				dest.Set(fieldDesc, cloneValue(srcVal, fieldDesc.Kind()))
			default:
				// only singular message field can be merged partially
				submask.mergeImpl(srcVal.Message(), dest.Mutable(fieldDesc).Message())
			}

		default:
			dest.Set(fieldDesc, srcVal) // scalar value
		}
	}
}

// cloneValue returns a cloned value for message kind or the same instance of
// input value for all the other kinds (i.e. scalar). List and map value are not
// expected as they have been explicitly handled in mergeImpl.
func cloneValue(v protoreflect.Value, kind protoreflect.Kind) protoreflect.Value {
	if kind == protoreflect.MessageKind {
		clonedMsg := proto.Clone(v.Message().Interface()).ProtoReflect()
		return protoreflect.ValueOf(clonedMsg)
	}
	return v
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
// mask.
func (m *Mask) Submask(path string) (*Mask, error) {
	ctx := &parseCtx{
		curDescriptor: m.descriptor,
		allowAdvanced: m.isAdvanced,
		isList:        m.isRepeated && !(m.descriptor != nil && m.descriptor.IsMapEntry()),
	}

	parsedPath, err := parsePathWithContext(path, ctx)
	if err != nil {
		return nil, err
	}
	switch incl, inclMask := m.includesImpl(parsedPath); incl {
	case IncludeEntirely:
		return &Mask{
			descriptor: ctx.curDescriptor,
			isRepeated: ctx.isList || (ctx.curDescriptor != nil && ctx.curDescriptor.IsMapEntry()),
			isAdvanced: m.isAdvanced,
		}, nil
	case Exclude:
		return nil, fmt.Errorf("the given path %q is excluded from mask", path)
	case IncludePartially:
		return inclMask, nil
	default:
		return nil, fmt.Errorf("unknown Inclusiveness: %d", incl)
	}
}

// MustSubmask returns a sub-mask given a path from the received mask to it.
//
// This is essentially the same as Submask, but panics if the given path is invalid or
// exlcuded from the received mask.
func (m *Mask) MustSubmask(path string) *Mask {
	sm, err := m.Submask(path)
	if err != nil {
		panic(fmt.Sprintf("MustSubmask(%q): %s", path, err))
	}
	return sm
}

// IsEmpty reports whether a mask is of empty value. Such mask implies keeping
// everything when calling Trim, merging nothing when calling Merge and always
// returning IncludeEntirely when calling Includes
func (m *Mask) IsEmpty() bool {
	return m == nil || (m.descriptor == nil && !m.isRepeated && len(m.children) == 0)
}

// checkMsgHaveDesc validates that the descriptor of given proto message
// matches the expected message descriptor. It returns an error when the given
// message is nil or descriptor of which doesn't match the expectation.
func checkMsgHaveDesc(msg proto.Message, expectedDesc protoreflect.MessageDescriptor) (protoreflect.Message, error) {
	if msg == nil {
		return nil, fmt.Errorf("nil message")
	}
	refl := msg.ProtoReflect()
	if msgDesc := refl.Descriptor(); msgDesc != expectedDesc {
		return nil, fmt.Errorf("expected message have descriptor: %s; got descriptor: %s", expectedDesc.FullName(), msgDesc.FullName())
	}
	return refl, nil
}
