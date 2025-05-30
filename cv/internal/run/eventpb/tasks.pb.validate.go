// Code generated by protoc-gen-validate. DO NOT EDIT.
// source: go.chromium.org/luci/cv/internal/run/eventpb/tasks.proto

package eventpb

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"google.golang.org/protobuf/types/known/anypb"
)

// ensure the imports are used
var (
	_ = bytes.MinRead
	_ = errors.New("")
	_ = fmt.Print
	_ = utf8.UTFMax
	_ = (*regexp.Regexp)(nil)
	_ = (*strings.Reader)(nil)
	_ = net.IPv4len
	_ = time.Duration(0)
	_ = (*url.URL)(nil)
	_ = (*mail.Address)(nil)
	_ = anypb.Any{}
	_ = sort.Sort
)

// Validate checks the field values on ManageRunTask with the rules defined in
// the proto definition for this message. If any rules are violated, the first
// error encountered is returned, or nil if there are no violations.
func (m *ManageRunTask) Validate() error {
	return m.validate(false)
}

// ValidateAll checks the field values on ManageRunTask with the rules defined
// in the proto definition for this message. If any rules are violated, the
// result is a list of violation errors wrapped in ManageRunTaskMultiError, or
// nil if none found.
func (m *ManageRunTask) ValidateAll() error {
	return m.validate(true)
}

func (m *ManageRunTask) validate(all bool) error {
	if m == nil {
		return nil
	}

	var errors []error

	// no validation rules for RunId

	if len(errors) > 0 {
		return ManageRunTaskMultiError(errors)
	}

	return nil
}

// ManageRunTaskMultiError is an error wrapping multiple validation errors
// returned by ManageRunTask.ValidateAll() if the designated constraints
// aren't met.
type ManageRunTaskMultiError []error

// Error returns a concatenation of all the error messages it wraps.
func (m ManageRunTaskMultiError) Error() string {
	msgs := make([]string, 0, len(m))
	for _, err := range m {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// AllErrors returns a list of validation violation errors.
func (m ManageRunTaskMultiError) AllErrors() []error { return m }

// ManageRunTaskValidationError is the validation error returned by
// ManageRunTask.Validate if the designated constraints aren't met.
type ManageRunTaskValidationError struct {
	field  string
	reason string
	cause  error
	key    bool
}

// Field function returns field value.
func (e ManageRunTaskValidationError) Field() string { return e.field }

// Reason function returns reason value.
func (e ManageRunTaskValidationError) Reason() string { return e.reason }

// Cause function returns cause value.
func (e ManageRunTaskValidationError) Cause() error { return e.cause }

// Key function returns key value.
func (e ManageRunTaskValidationError) Key() bool { return e.key }

// ErrorName returns error name.
func (e ManageRunTaskValidationError) ErrorName() string { return "ManageRunTaskValidationError" }

// Error satisfies the builtin error interface
func (e ManageRunTaskValidationError) Error() string {
	cause := ""
	if e.cause != nil {
		cause = fmt.Sprintf(" | caused by: %v", e.cause)
	}

	key := ""
	if e.key {
		key = "key for "
	}

	return fmt.Sprintf(
		"invalid %sManageRunTask.%s: %s%s",
		key,
		e.field,
		e.reason,
		cause)
}

var _ error = ManageRunTaskValidationError{}

var _ interface {
	Field() string
	Reason() string
	Key() bool
	Cause() error
	ErrorName() string
} = ManageRunTaskValidationError{}

// Validate checks the field values on KickManageRunTask with the rules defined
// in the proto definition for this message. If any rules are violated, the
// first error encountered is returned, or nil if there are no violations.
func (m *KickManageRunTask) Validate() error {
	return m.validate(false)
}

// ValidateAll checks the field values on KickManageRunTask with the rules
// defined in the proto definition for this message. If any rules are
// violated, the result is a list of violation errors wrapped in
// KickManageRunTaskMultiError, or nil if none found.
func (m *KickManageRunTask) ValidateAll() error {
	return m.validate(true)
}

func (m *KickManageRunTask) validate(all bool) error {
	if m == nil {
		return nil
	}

	var errors []error

	// no validation rules for RunId

	if all {
		switch v := interface{}(m.GetEta()).(type) {
		case interface{ ValidateAll() error }:
			if err := v.ValidateAll(); err != nil {
				errors = append(errors, KickManageRunTaskValidationError{
					field:  "Eta",
					reason: "embedded message failed validation",
					cause:  err,
				})
			}
		case interface{ Validate() error }:
			if err := v.Validate(); err != nil {
				errors = append(errors, KickManageRunTaskValidationError{
					field:  "Eta",
					reason: "embedded message failed validation",
					cause:  err,
				})
			}
		}
	} else if v, ok := interface{}(m.GetEta()).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return KickManageRunTaskValidationError{
				field:  "Eta",
				reason: "embedded message failed validation",
				cause:  err,
			}
		}
	}

	if len(errors) > 0 {
		return KickManageRunTaskMultiError(errors)
	}

	return nil
}

// KickManageRunTaskMultiError is an error wrapping multiple validation errors
// returned by KickManageRunTask.ValidateAll() if the designated constraints
// aren't met.
type KickManageRunTaskMultiError []error

// Error returns a concatenation of all the error messages it wraps.
func (m KickManageRunTaskMultiError) Error() string {
	msgs := make([]string, 0, len(m))
	for _, err := range m {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// AllErrors returns a list of validation violation errors.
func (m KickManageRunTaskMultiError) AllErrors() []error { return m }

// KickManageRunTaskValidationError is the validation error returned by
// KickManageRunTask.Validate if the designated constraints aren't met.
type KickManageRunTaskValidationError struct {
	field  string
	reason string
	cause  error
	key    bool
}

// Field function returns field value.
func (e KickManageRunTaskValidationError) Field() string { return e.field }

// Reason function returns reason value.
func (e KickManageRunTaskValidationError) Reason() string { return e.reason }

// Cause function returns cause value.
func (e KickManageRunTaskValidationError) Cause() error { return e.cause }

// Key function returns key value.
func (e KickManageRunTaskValidationError) Key() bool { return e.key }

// ErrorName returns error name.
func (e KickManageRunTaskValidationError) ErrorName() string {
	return "KickManageRunTaskValidationError"
}

// Error satisfies the builtin error interface
func (e KickManageRunTaskValidationError) Error() string {
	cause := ""
	if e.cause != nil {
		cause = fmt.Sprintf(" | caused by: %v", e.cause)
	}

	key := ""
	if e.key {
		key = "key for "
	}

	return fmt.Sprintf(
		"invalid %sKickManageRunTask.%s: %s%s",
		key,
		e.field,
		e.reason,
		cause)
}

var _ error = KickManageRunTaskValidationError{}

var _ interface {
	Field() string
	Reason() string
	Key() bool
	Cause() error
	ErrorName() string
} = KickManageRunTaskValidationError{}

// Validate checks the field values on ManageRunLongOpTask with the rules
// defined in the proto definition for this message. If any rules are
// violated, the first error encountered is returned, or nil if there are no violations.
func (m *ManageRunLongOpTask) Validate() error {
	return m.validate(false)
}

// ValidateAll checks the field values on ManageRunLongOpTask with the rules
// defined in the proto definition for this message. If any rules are
// violated, the result is a list of violation errors wrapped in
// ManageRunLongOpTaskMultiError, or nil if none found.
func (m *ManageRunLongOpTask) ValidateAll() error {
	return m.validate(true)
}

func (m *ManageRunLongOpTask) validate(all bool) error {
	if m == nil {
		return nil
	}

	var errors []error

	// no validation rules for RunId

	// no validation rules for OperationId

	if len(errors) > 0 {
		return ManageRunLongOpTaskMultiError(errors)
	}

	return nil
}

// ManageRunLongOpTaskMultiError is an error wrapping multiple validation
// errors returned by ManageRunLongOpTask.ValidateAll() if the designated
// constraints aren't met.
type ManageRunLongOpTaskMultiError []error

// Error returns a concatenation of all the error messages it wraps.
func (m ManageRunLongOpTaskMultiError) Error() string {
	msgs := make([]string, 0, len(m))
	for _, err := range m {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// AllErrors returns a list of validation violation errors.
func (m ManageRunLongOpTaskMultiError) AllErrors() []error { return m }

// ManageRunLongOpTaskValidationError is the validation error returned by
// ManageRunLongOpTask.Validate if the designated constraints aren't met.
type ManageRunLongOpTaskValidationError struct {
	field  string
	reason string
	cause  error
	key    bool
}

// Field function returns field value.
func (e ManageRunLongOpTaskValidationError) Field() string { return e.field }

// Reason function returns reason value.
func (e ManageRunLongOpTaskValidationError) Reason() string { return e.reason }

// Cause function returns cause value.
func (e ManageRunLongOpTaskValidationError) Cause() error { return e.cause }

// Key function returns key value.
func (e ManageRunLongOpTaskValidationError) Key() bool { return e.key }

// ErrorName returns error name.
func (e ManageRunLongOpTaskValidationError) ErrorName() string {
	return "ManageRunLongOpTaskValidationError"
}

// Error satisfies the builtin error interface
func (e ManageRunLongOpTaskValidationError) Error() string {
	cause := ""
	if e.cause != nil {
		cause = fmt.Sprintf(" | caused by: %v", e.cause)
	}

	key := ""
	if e.key {
		key = "key for "
	}

	return fmt.Sprintf(
		"invalid %sManageRunLongOpTask.%s: %s%s",
		key,
		e.field,
		e.reason,
		cause)
}

var _ error = ManageRunLongOpTaskValidationError{}

var _ interface {
	Field() string
	Reason() string
	Key() bool
	Cause() error
	ErrorName() string
} = ManageRunLongOpTaskValidationError{}
