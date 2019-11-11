package main

import (
	"path"
	"strings"
)

// Path is a file path.
type Path []string

func (p Path) String() string {
	return path.Join(p...)
}

// Split returns path to the parent and the base name.
// If p has only one component, returns nil and the component.
// If p is empty, panics.
func (p Path) Split() (parent Path, base string) {
	switch len(p) {
	case 0:
		panic("p is empty")
	case 1:
		return nil, p[0]
	default:
		last := len(p) - 1
		return Path(p[:last]), p[last]
	}
}

// ParsePath parses a slash-separatd path.
func ParsePath(slashSeparated string) Path {
	return Path(strings.Split(slashSeparated, "/"))
}
