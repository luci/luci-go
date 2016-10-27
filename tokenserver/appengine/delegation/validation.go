// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"fmt"
	"strings"

	"github.com/luci/luci-go/common/data/stringset"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/server/auth/identity"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
)

// ValidateConfig checks delegation config for correctness.
//
// Tries to find all errors.
func ValidateConfig(cfg *admin.DelegationPermissions) errors.MultiError {
	var errs errors.MultiError
	names := stringset.New(0)

	for i, rule := range cfg.Rules {
		prefix := fmt.Sprintf("rule #%d (%q)", i+1, rule.Name)
		if rule.Name != "" {
			if names.Has(rule.Name) {
				errs = append(errs, fmt.Errorf("%s: the rule with such name is already defined", prefix))
			}
			names.Add(rule.Name)
		}
		for _, singleErr := range ValidateRule(rule) {
			errs = append(errs, fmt.Errorf("%s: %s", prefix, singleErr))
		}
	}

	return errs
}

// ValidateRule checks single DelegationRule proto.
//
// See config.proto, DelegationRule for the description of allowed values.
func ValidateRule(r *admin.DelegationRule) errors.MultiError {
	var out errors.MultiError

	emitErr := func(msg string, args ...interface{}) {
		out = append(out, fmt.Errorf(msg, args...))
	}

	emitMultiErr := func(prefix string, merr errors.MultiError) {
		for _, err := range merr {
			emitErr("%s - %s", prefix, err)
		}
	}

	if r.Name == "" {
		emitErr("'name' is required")
	}

	if len(r.Requestor) == 0 {
		emitErr("'requestor' is required")
	} else {
		v := identitySetValidator{
			AllowGroups: true,
		}
		emitMultiErr("bad 'requestor'", v.Validate(r.Requestor))
	}

	if len(r.AllowedToImpersonate) == 0 {
		emitErr("'allowed_to_impersonate' is required")
	} else {
		v := identitySetValidator{
			AllowReservedWords: []string{Requestor}, // '*' is not allowed here though
			AllowGroups:        true,
		}
		emitMultiErr("bad 'allowed_to_impersonate'", v.Validate(r.AllowedToImpersonate))
	}

	if len(r.AllowedAudience) == 0 {
		emitErr("'allowed_audience' is required")
	} else {
		v := identitySetValidator{
			AllowReservedWords: []string{Requestor, "*"},
			AllowGroups:        true,
		}
		emitMultiErr("bad 'allowed_audience'", v.Validate(r.AllowedAudience))
	}

	if len(r.TargetService) == 0 {
		emitErr("'target_service' is required")
	} else {
		v := identitySetValidator{
			AllowReservedWords: []string{"*"},
			AllowIDKinds:       []identity.Kind{identity.Service},
		}
		emitMultiErr("bad 'target_service'", v.Validate(r.TargetService))
	}

	if r.MaxValidityDuration == 0 {
		emitErr("'max_validity_duration' is required")
	}
	if r.MaxValidityDuration < 0 {
		emitErr("'max_validity_duration' must be positive")
	}
	if r.MaxValidityDuration > 24*3600 {
		emitErr("'max_validity_duration' must be smaller than 86401")
	}

	return out
}

type identitySetValidator struct {
	AllowReservedWords []string        // to allow "*" and "REQUESTOR"
	AllowGroups        bool            // true to allow "group:" entries
	AllowIDKinds       []identity.Kind // permitted identity kinds, or nil if all
}

func (v *identitySetValidator) Validate(items []string) errors.MultiError {
	var out errors.MultiError

	emitErr := func(msg string, args ...interface{}) {
		out = append(out, fmt.Errorf(msg, args...))
	}

loop:
	for _, s := range items {
		// A reserved word?
		for _, r := range v.AllowReservedWords {
			if s == r {
				continue loop
			}
		}

		// A group reference?
		if strings.HasPrefix(s, "group:") {
			if !v.AllowGroups {
				emitErr("group entries are not allowed - %q", s)
			} else {
				if s == "group:" {
					emitErr("bad group entry %q", s)
				}
			}
			continue
		}

		// An identity then.
		id, err := identity.MakeIdentity(s)
		if err != nil {
			emitErr("%s", err)
			continue
		}

		if v.AllowIDKinds != nil {
			allowed := false
			for _, k := range v.AllowIDKinds {
				if id.Kind() == k {
					allowed = true
					break
				}
			}
			if !allowed {
				emitErr("identity of kind %q is not allowed here - %q", id.Kind(), s)
			}
		}
	}

	return out
}
