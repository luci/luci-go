package gitiles

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors"
)

// Validate returns an error if r is invalid.
func (r *LogRequest) Validate() error {
	switch {
	case r.Project == "":
		return errors.New("project is required")
	case r.PageSize < 0:
		return errors.New("page size must not be negative")
	case r.Treeish == "":
		return errors.New("treeish is required")
	default:
		return nil
	}
}

// Validate returns an error if r is invalid.
func (r *RefsRequest) Validate() error {
	switch {
	case r.Project == "":
		return errors.New("project is required")
	case r.RefsPath != "refs" && !strings.HasPrefix(r.RefsPath, "refs/"):
		return fmt.Errorf(`refsPath must be "refs" or start with "refs/": %q`, r.RefsPath)
	default:
		return nil
	}
}
