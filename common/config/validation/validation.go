// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package validation provides a helper for performing config validations.
package validation

import (
	"fmt"
	"strings"

	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
)

// Error is an error with details of failed validation.
//
// Returned by Context.Finalize().
type Error struct {
	// Errors is a list of individual validation errors.
	//
	// Each one is annotated with "file" string and a logical path pointing to
	// the element that contains the error. It is provided as a slice of strings
	// in "element" annotation.
	Errors errors.MultiError
}

// Error makes *Error implement 'error' interface.
func (v *Error) Error() string {
	return v.Errors.Error()
}

// Context is an accumulator for validation errors.
//
// It is passed to a function that does config validation. Such function may
// validate a bunch of files (using SetFile to indicate which one is processed
// now). Each file may have some internal nested structure. The logical path
// inside this structure is captured through Enter and Exit calls.
type Context struct {
	Logger logging.Logger // logs errors as they appear

	errors  errors.MultiError // all accumulated errors
	file    string            // the currently validated file
	element []string          // logical path of a sub-element we validate, see Enter
}

type fileTagType struct{ Key errors.TagKey }

func (f fileTagType) With(name string) errors.TagValue {
	return errors.TagValue{Key: f.Key, Value: name}
}
func (f fileTagType) In(err error) (v string, ok bool) {
	d, ok := errors.TagValueIn(f.Key, err)
	if ok {
		v = d.(string)
	}
	return
}

type elementTagType struct{ Key errors.TagKey }

func (e elementTagType) With(elements []string) errors.TagValue {
	return errors.TagValue{Key: e.Key, Value: append([]string(nil), elements...)}
}
func (e elementTagType) In(err error) (v []string, ok bool) {
	d, ok := errors.TagValueIn(e.Key, err)
	if ok {
		v = d.([]string)
	}
	return
}

var fileTag = fileTagType{errors.NewTagKey("holds the file name for tests")}
var elementTag = elementTagType{errors.NewTagKey("holds the elements for tests")}

// Error records a validation error.
func (v *Context) Error(msg string, args ...interface{}) {
	ctx := ""
	if v.file != "" {
		ctx = fmt.Sprintf("in %q", v.file)
	} else {
		ctx = "in <unspecified file>"
	}
	if len(v.element) != 0 {
		ctx += " (" + strings.Join(v.element, " / ") + ")"
	}

	// Prepending ctx to msg before passing it to fmt is not entirely correct,
	// since ctx may have format specifiers (like %s), that will be misunderstood.
	// So we put ctx in the argument list.
	msg = "%s: " + msg
	args = append([]interface{}{ctx}, args...)
	if v.Logger != nil {
		v.Logger.Errorf(msg, args...)
	}

	// Make the file and the logical path also usable through error inspection.
	err := errors.Reason(fmt.Sprintf(msg, args...)).
		Tag(fileTag.With(v.file), elementTag.With(v.element)).Err()
	v.errors = append(v.errors, err)
}

// SetFile records that what follows is errors for this particular file.
//
// Changing the file resets the current element (see Enter/Exit).
func (v *Context) SetFile(path string) {
	if v.file != path {
		v.file = path
		v.element = nil
	}
}

// Enter descends into a sub-element when validating a nested structure.
//
// Useful for defining context. A current path of elements shows up in
// validation messages.
//
// The reverse is Exit.
func (v *Context) Enter(title string, args ...interface{}) {
	e := fmt.Sprintf(title, args...)
	v.element = append(v.element, e)
}

// Exit pops the current element we are visiting from the stack.
//
// This is the reverse of Enter. Each Enter must have corresponding Exit. Use
// functions and defers to ensure this, if it's otherwise hard to track.
func (v *Context) Exit() {
	if len(v.element) != 0 {
		v.element = v.element[:len(v.element)-1]
	}
}

// Finalize returns *Error if some validation errors were recorded.
//
// Returns nil otherwise.
func (v *Context) Finalize() error {
	if len(v.errors) == 0 {
		return nil
	}
	return &Error{
		Errors: append(errors.MultiError{}, v.errors...),
	}
}
