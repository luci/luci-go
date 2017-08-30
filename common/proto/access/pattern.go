package access

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var (
	rgxKind  = regexp.MustCompile(`^[a-z\-]+$`)
	rgxParam = regexp.MustCompile(`^\{[a-z\-]+}$`)
)

type patternTuple struct {
	kind      string
	parameter string
}

func parsePattern(pattern string) ([]patternTuple, error) {
	parts := strings.Split(pattern, "/")
	// note: len(strings.Split("", "/")) == 1
	if len(parts)%2 == 1 {
		return nil, errors.New("the number of slashes is not odd")
	}

	tuples := make([]patternTuple, len(parts)/2)
	seenParams := stringset.New(len(tuples))
	seenKinds := stringset.New(len(tuples))
	for i := 0; i < len(parts); i += 2 {
		kind, param := parts[i], parts[i+1]
		if !rgxKind.MatchString(kind) {
			return nil, errors.Reason("kind %q does not match regex %q", kind, rgxKind).Err()
		}
		if !rgxParam.MatchString(param) {
			return nil, errors.Reason("parameter %q does not match regex %q", param, rgxParam).Err()
		}
		param = strings.Trim(param, "{}")

		if !seenKinds.Add(kind) {
			return nil, errors.Reason("duplicate kind %q", kind).Err()
		}
		if !seenParams.Add(param) {
			return nil, errors.Reason("duplicate parameter %q", param).Err()
		}

		tuples[i/2] = patternTuple{kind, param}
	}
	return tuples, nil
}

type patternNode struct {
	children map[string]*patternNode
	res      *Resource
}

// add adds r to the tree.
// Assumes r.pattern is initialized.
func (n *patternNode) add(r *Resource) error {
	cur := n
	kinds := make([]string, len(r.pattern))
	for i, t := range r.pattern {
		kinds[i] = t.kind
		next := cur.children[t.kind]
		if next == nil {
			next = &patternNode{}
			if cur.children == nil {
				cur.children = map[string]*patternNode{}
			}
			cur.children[t.kind] = next
		}
		cur = next
	}

	if cur.res != nil {
		return fmt.Errorf("a resource with kinds %q already exists", kinds)
	}
	cur.res = r
	return nil
}

func (n *patternNode) resolve(resource string) (res *Resource, args []string, err error) {
	parts := strings.Split(resource, "/")
	// note: len(strings.Split("", "/")) == 1
	if len(parts)%2 == 1 {
		return nil, nil, errors.Reason("invalid resource %q", resource).Err()
	}

	cur := n
	args = make([]string, 0, len(parts)/2)
	for i := 0; i < len(parts); i += 2 {
		kind, arg := parts[0], parts[1]
		if v, err := url.PathUnescape(arg); err != nil {
			return nil, nil, errors.Annotate(err, "invalid arg %q", arg).Err()
		} else {
			arg = v
		}
		args = append(args, arg)
		cur = cur.children[kind]
		if cur == nil {
			return nil, nil, errors.Reason("invalid resource %q", resource).Err()
		}
	}
	res = cur.res
	return
}
