// Code generated by protoc-gen-validate. DO NOT EDIT.
// source: go.chromium.org/luci/server/quota/quotapb/policy.proto

package quotapb

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

// Validate checks the field values on Policy with the rules defined in the
// proto definition for this message. If any rules are violated, the first
// error encountered is returned, or nil if there are no violations.
func (m *Policy) Validate() error {
	return m.validate(false)
}

// ValidateAll checks the field values on Policy with the rules defined in the
// proto definition for this message. If any rules are violated, the result is
// a list of violation errors wrapped in PolicyMultiError, or nil if none found.
func (m *Policy) ValidateAll() error {
	return m.validate(true)
}

func (m *Policy) validate(all bool) error {
	if m == nil {
		return nil
	}

	var errors []error

	if m.GetDefault() > 9007199254740991 {
		err := PolicyValidationError{
			field:  "Default",
			reason: "value must be less than or equal to 9007199254740991",
		}
		if !all {
			return err
		}
		errors = append(errors, err)
	}

	if m.GetLimit() > 9007199254740991 {
		err := PolicyValidationError{
			field:  "Limit",
			reason: "value must be less than or equal to 9007199254740991",
		}
		if !all {
			return err
		}
		errors = append(errors, err)
	}

	if all {
		switch v := interface{}(m.GetRefill()).(type) {
		case interface{ ValidateAll() error }:
			if err := v.ValidateAll(); err != nil {
				errors = append(errors, PolicyValidationError{
					field:  "Refill",
					reason: "embedded message failed validation",
					cause:  err,
				})
			}
		case interface{ Validate() error }:
			if err := v.Validate(); err != nil {
				errors = append(errors, PolicyValidationError{
					field:  "Refill",
					reason: "embedded message failed validation",
					cause:  err,
				})
			}
		}
	} else if v, ok := interface{}(m.GetRefill()).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return PolicyValidationError{
				field:  "Refill",
				reason: "embedded message failed validation",
				cause:  err,
			}
		}
	}

	// no validation rules for Options

	if all {
		switch v := interface{}(m.GetLifetime()).(type) {
		case interface{ ValidateAll() error }:
			if err := v.ValidateAll(); err != nil {
				errors = append(errors, PolicyValidationError{
					field:  "Lifetime",
					reason: "embedded message failed validation",
					cause:  err,
				})
			}
		case interface{ Validate() error }:
			if err := v.Validate(); err != nil {
				errors = append(errors, PolicyValidationError{
					field:  "Lifetime",
					reason: "embedded message failed validation",
					cause:  err,
				})
			}
		}
	} else if v, ok := interface{}(m.GetLifetime()).(interface{ Validate() error }); ok {
		if err := v.Validate(); err != nil {
			return PolicyValidationError{
				field:  "Lifetime",
				reason: "embedded message failed validation",
				cause:  err,
			}
		}
	}

	if len(errors) > 0 {
		return PolicyMultiError(errors)
	}

	return nil
}

// PolicyMultiError is an error wrapping multiple validation errors returned by
// Policy.ValidateAll() if the designated constraints aren't met.
type PolicyMultiError []error

// Error returns a concatenation of all the error messages it wraps.
func (m PolicyMultiError) Error() string {
	var msgs []string
	for _, err := range m {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// AllErrors returns a list of validation violation errors.
func (m PolicyMultiError) AllErrors() []error { return m }

// PolicyValidationError is the validation error returned by Policy.Validate if
// the designated constraints aren't met.
type PolicyValidationError struct {
	field  string
	reason string
	cause  error
	key    bool
}

// Field function returns field value.
func (e PolicyValidationError) Field() string { return e.field }

// Reason function returns reason value.
func (e PolicyValidationError) Reason() string { return e.reason }

// Cause function returns cause value.
func (e PolicyValidationError) Cause() error { return e.cause }

// Key function returns key value.
func (e PolicyValidationError) Key() bool { return e.key }

// ErrorName returns error name.
func (e PolicyValidationError) ErrorName() string { return "PolicyValidationError" }

// Error satisfies the builtin error interface
func (e PolicyValidationError) Error() string {
	cause := ""
	if e.cause != nil {
		cause = fmt.Sprintf(" | caused by: %v", e.cause)
	}

	key := ""
	if e.key {
		key = "key for "
	}

	return fmt.Sprintf(
		"invalid %sPolicy.%s: %s%s",
		key,
		e.field,
		e.reason,
		cause)
}

var _ error = PolicyValidationError{}

var _ interface {
	Field() string
	Reason() string
	Key() bool
	Cause() error
	ErrorName() string
} = PolicyValidationError{}

// Validate checks the field values on Policy_Refill with the rules defined in
// the proto definition for this message. If any rules are violated, the first
// error encountered is returned, or nil if there are no violations.
func (m *Policy_Refill) Validate() error {
	return m.validate(false)
}

// ValidateAll checks the field values on Policy_Refill with the rules defined
// in the proto definition for this message. If any rules are violated, the
// result is a list of violation errors wrapped in Policy_RefillMultiError, or
// nil if none found.
func (m *Policy_Refill) ValidateAll() error {
	return m.validate(true)
}

func (m *Policy_Refill) validate(all bool) error {
	if m == nil {
		return nil
	}

	var errors []error

	if val := m.GetUnits(); val < -9007199254740991 || val > 9007199254740991 {
		err := Policy_RefillValidationError{
			field:  "Units",
			reason: "value must be inside range [-9007199254740991, 9007199254740991]",
		}
		if !all {
			return err
		}
		errors = append(errors, err)
	}

	if _, ok := _Policy_Refill_Units_NotInLookup[m.GetUnits()]; ok {
		err := Policy_RefillValidationError{
			field:  "Units",
			reason: "value must not be in list [0]",
		}
		if !all {
			return err
		}
		errors = append(errors, err)
	}

	if m.GetInterval() > 86400 {
		err := Policy_RefillValidationError{
			field:  "Interval",
			reason: "value must be less than or equal to 86400",
		}
		if !all {
			return err
		}
		errors = append(errors, err)
	}

	if m.GetOffset() > 86400 {
		err := Policy_RefillValidationError{
			field:  "Offset",
			reason: "value must be less than or equal to 86400",
		}
		if !all {
			return err
		}
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return Policy_RefillMultiError(errors)
	}

	return nil
}

// Policy_RefillMultiError is an error wrapping multiple validation errors
// returned by Policy_Refill.ValidateAll() if the designated constraints
// aren't met.
type Policy_RefillMultiError []error

// Error returns a concatenation of all the error messages it wraps.
func (m Policy_RefillMultiError) Error() string {
	var msgs []string
	for _, err := range m {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "; ")
}

// AllErrors returns a list of validation violation errors.
func (m Policy_RefillMultiError) AllErrors() []error { return m }

// Policy_RefillValidationError is the validation error returned by
// Policy_Refill.Validate if the designated constraints aren't met.
type Policy_RefillValidationError struct {
	field  string
	reason string
	cause  error
	key    bool
}

// Field function returns field value.
func (e Policy_RefillValidationError) Field() string { return e.field }

// Reason function returns reason value.
func (e Policy_RefillValidationError) Reason() string { return e.reason }

// Cause function returns cause value.
func (e Policy_RefillValidationError) Cause() error { return e.cause }

// Key function returns key value.
func (e Policy_RefillValidationError) Key() bool { return e.key }

// ErrorName returns error name.
func (e Policy_RefillValidationError) ErrorName() string { return "Policy_RefillValidationError" }

// Error satisfies the builtin error interface
func (e Policy_RefillValidationError) Error() string {
	cause := ""
	if e.cause != nil {
		cause = fmt.Sprintf(" | caused by: %v", e.cause)
	}

	key := ""
	if e.key {
		key = "key for "
	}

	return fmt.Sprintf(
		"invalid %sPolicy_Refill.%s: %s%s",
		key,
		e.field,
		e.reason,
		cause)
}

var _ error = Policy_RefillValidationError{}

var _ interface {
	Field() string
	Reason() string
	Key() bool
	Cause() error
	ErrorName() string
} = Policy_RefillValidationError{}

var _Policy_Refill_Units_NotInLookup = map[int64]struct{}{
	0: {},
}