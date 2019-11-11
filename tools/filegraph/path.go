package main

import (
	"path"
	"strings"
)

// Path is a path to a file with a repository.
// It is an ordered list of path components relative to the repository root.
type Path []string

// String returns string representation of the path, where components are
// separated with forward slashes.
func (p Path) String() string {
	return path.Join(p...)
}

// ParsePath parses a slash-separatd path.
func ParsePath(slashSeparated string) Path {
	return Path(strings.Split(slashSeparated, "/"))
}
