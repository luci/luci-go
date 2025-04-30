// Copyright 2017 The LUCI Authors.
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

// Package validation provides a helper for performing config validations.
package validation

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/logging"
	configpb "go.chromium.org/luci/common/proto/config"
)

// Error is an error with details of validation issues.
//
// Returned by Context.Finalize().
type Error struct {
	// Errors is a list of individual validation errors.
	//
	// Each one is annotated with "file" string, logical path pointing to
	// the element that contains the error, and its severity. It is provided as a
	// slice of strings in "element" annotation.
	Errors errors.MultiError
}

// Error makes *Error implement 'error' interface.
func (e *Error) Error() string {
	return e.Errors.Error()
}

// WithSeverity returns a multi-error with errors of a given severity only.
func (e *Error) WithSeverity(s Severity) error {
	var filtered errors.MultiError
	for _, valErr := range e.Errors {
		if severity, ok := SeverityTag.Value(valErr); ok && severity == s {
			filtered = append(filtered, valErr)
		}
	}
	if len(filtered) != 0 {
		return filtered
	}
	return nil
}

// ToValidationResultMsgs converts `Error` to a slice of
// `configpb.ValidationResult.Message`s.
func (e *Error) ToValidationResultMsgs(ctx context.Context) []*configpb.ValidationResult_Message {
	if e == nil || len(e.Errors) == 0 {
		return nil
	}
	ret := make([]*configpb.ValidationResult_Message, len(e.Errors))
	for i, err := range e.Errors {
		// validation.Context supports just 2 severities now,
		// but defensively default to ERROR level in unexpected cases.
		msgSeverity := configpb.ValidationResult_ERROR
		switch severity, ok := SeverityTag.Value(err); {
		case !ok:
			logging.Errorf(ctx, "unset validation.Severity in %s", err)
		case severity == Warning:
			msgSeverity = configpb.ValidationResult_WARNING
		case severity != Blocking:
			logging.Errorf(ctx, "unrecognized validation.Severity %d in %s", severity, err)
		}
		file, ok := fileTag.Value(err)
		if !ok || file == "" {
			file = "unspecified file"
		}
		ret[i] = &configpb.ValidationResult_Message{
			Path:     file,
			Severity: msgSeverity,
			Text:     err.Error(),
		}
	}
	return ret
}

// Context is an accumulator for validation errors.
//
// It is passed to a function that does config validation. Such function may
// validate a bunch of files (using SetFile to indicate which one is processed
// now). Each file may have some internal nested structure. The logical path
// inside this structure is captured through Enter and Exit calls.
type Context struct {
	Context context.Context

	errors  errors.MultiError // all accumulated errors, including those with Warning severity.
	file    string            // the currently validated file
	element []string          // logical path of a sub-element we validate, see Enter
}

// Severity of the validation message.
//
// Only Blocking and Warning severities are supported.
type Severity int

const (
	// Blocking severity blocks config from being accepted.
	//
	// Corresponds to ValidationResponseMessage_Severity:ERROR.
	Blocking Severity = 0
	// Warning severity doesn't block config from being accepted.
	//
	// Corresponds to ValidationResponseMessage_Severity:WARNING.
	Warning Severity = 1
)

// TODO: consider combining these tags into a single errorContext struct/tag.

var fileTag = errtag.Make("holds the file name for tests", "")
var elementTag = errtag.Make("holds the elements for tests", (*[]string)(nil))

// SeverityTag holds the severity of the given validation error.
var SeverityTag = errtag.Make("holds the severity", Blocking)

// Errorf records the given format string and args as a blocking validation error.
func (v *Context) Errorf(format string, args ...any) {
	v.record(Blocking, errors.Reason(format, args...).Err())
}

// Error records the given error as a blocking validation error.
func (v *Context) Error(err error) {
	v.record(Blocking, err)
}

// HasPendingErrors returns true if there's any pending validation errors.
func (v *Context) HasPendingErrors() bool {
	return v.hasErrors(Blocking)
}

// Warningf records the given format string and args as a validation warning.
func (v *Context) Warningf(format string, args ...any) {
	v.record(Warning, errors.Reason(format, args...).Err())
}

// Warning records the given error as a validation warning.
func (v *Context) Warning(err error) {
	v.record(Warning, err)
}

// HasPendingWarnings returns true if there's any pending validation warnings.
func (v *Context) HasPendingWarnings() bool {
	return v.hasErrors(Warning)
}

func (v *Context) record(severity Severity, err error) {
	if err == nil {
		return
	}

	ctx := ""
	if v.file != "" {
		ctx = fmt.Sprintf("in %q", v.file)
	} else {
		ctx = "in <unspecified file>"
	}
	if len(v.element) != 0 {
		ctx += " (" + strings.Join(v.element, " / ") + ")"
	}
	// Make the file and the logical path also usable through error inspection.

	err = errors.Fmt("%s: %w", ctx, err)
	err = fileTag.ApplyValue(err, v.file)
	els := slices.Clone(v.element)
	err = elementTag.ApplyValue(err, &els)
	err = SeverityTag.ApplyValue(err, severity)
	v.errors = append(v.errors, err)
}

func (v *Context) hasErrors(severity Severity) bool {
	for _, err := range v.errors {
		if sev, ok := SeverityTag.Value(err); ok && sev == severity {
			return true
		}
	}
	return false
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
func (v *Context) Enter(title string, args ...any) {
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
