// Copyright 2021 The LUCI Authors.
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

package structmask

import (
	"encoding/json"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// pathElement represents a single parsed StructMask path element.
type pathElement any

type starElement struct{}
type fieldElement struct{ field string }
type indexElement struct{ index int }

// parseMask parses a mask into a filtering tree.
func parseMask(mask []*StructMask) (root *node, err error) {
	for _, m := range mask {
		elements := make([]pathElement, len(m.Path))
		for idx, p := range m.Path {
			if elements[idx], err = parseElement(p); err != nil {
				return nil, errors.Fmt("bad element %q in the mask %s: %w", p, maskToStr(m), err)
			}
		}
		if len(elements) == 0 {
			return nil, errors.New("bad empty mask")
		}
		if root, err = updateNode(root, elements); err != nil {
			return nil, errors.Fmt("unsupported mask %s: %w", maskToStr(m), err)
		}
	}
	return root, nil
}

// maskToStr converts a mask to a string for error messages.
func maskToStr(m *StructMask) string {
	blob, err := json.Marshal(m.Path)
	if err != nil {
		panic(err)
	}
	return string(blob)
}

// parseElement interprets one path element in a StructMask.
func parseElement(p string) (pathElement, error) {
	// If starts with `"` or `'` or ``` it must be a valid quoted string.
	if strings.HasPrefix(p, `"`) || strings.HasPrefix(p, `'`) || strings.HasPrefix(p, "`") {
		s, err := strconv.Unquote(p)
		if err != nil {
			return nil, errors.Fmt("bad quoted string: %w", err)
		}
		return fieldElement{s}, nil
	}

	// Reserve `/.../` for regexps, if we ever allow them.
	if len(p) >= 2 && strings.HasPrefix(p, `/`) && strings.HasSuffix(p, `/`) {
		return nil, errors.Fmt("regexp matches are not supported; "+
			"if you want to match a literal field /.../, wrap the value in quotes: %s",
			strconv.Quote(p))
	}

	// If it contains `*`, we require it to be just `*` for now. That way we can
	// later add prefix and suffix matches by allowing e.g. `something*`.
	if p == "*" {
		return starElement{}, nil
	}
	if strings.Contains(p, "*") {
		return nil, errors.Fmt("prefix and suffix matches are not supported; "+
			"if you want to match a field with literal `*` in it, wrap the value in quotes: %s",
			strconv.Quote(p))
	}

	// If it looks like a number (even a float), it is a list index. We require it
	// to be a non-negative integer though.
	if _, err := strconv.ParseFloat(p, 32); err == nil {
		val, err := strconv.ParseInt(p, 10, 32)
		if err != nil || val < 0 {
			return nil, errors.New("an index must be a non-negative integer")
		}
		return indexElement{int(val)}, nil
	}

	// Otherwise assume it is a field name.
	return fieldElement{p}, nil
}

// updateNode inserts the mask into the filtering tree.
//
// Empty `mask` here represent a leaf node ("take everything that's left").
func updateNode(n *node, mask []pathElement) (*node, error) {
	// If this is the very end of the mask, grab everything that's left.
	if len(mask) == 0 {
		return leafNode, nil
	}

	// Some mask path already requested this element and all its inner guts. Just
	// ignore `mask`, it will not filter out anything.
	if n == leafNode {
		return leafNode, nil
	}

	// `n == nil` happens when we visit some path for the first time ever. If `n`
	// is not nil, it means we've visited this path already in some previous
	// StructMask and just need to update it (e.g. add more fields).
	if n == nil {
		n = &node{}
	}

	var err error
	switch elem := mask[0].(type) {
	case starElement:
		n.star, err = updateNode(n.star, mask[1:])
	case fieldElement:
		if n.fields == nil {
			n.fields = make(map[string]*node, 1)
		}
		n.fields[elem.field], err = updateNode(n.fields[elem.field], mask[1:])
	case indexElement:
		err = errors.New("individual index selectors are not supported")
	}
	return n, err
}
