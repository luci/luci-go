package access

import (
	"regexp"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var (
	rgxActionID = regexp.MustCompile(`^[A-Z_]+$`)
	rgxRoleID   = rgxActionID
)

func (a *DescriptionResponse_ResourceDescription_Action) validate() error {
	if !rgxActionID.MatchString(a.ActionId) {
		return errors.Reason("action ID %q does not match regexp %q", a.ActionId, rgxActionID).Err()
	}
	return nil
}

func (r *DescriptionResponse_ResourceDescription_Role) validate(definedActions stringset.Set) error {
	if len(r.AllowedActions) == 0 {
		return errors.Reason("no actions are allowed").Err()
	}
	seenActions := stringset.New(len(r.AllowedActions))
	for _, a := range r.AllowedActions {
		if !definedActions.Has(a) {
			return errors.Reason("undefined action %q", a).Err()
		}
		if !seenActions.Add(a) {
			return errors.Reason("duplicate action %q", a).Err()
		}
	}
	return nil
}

// validate returns an error if d is invalid.
func (d *DescriptionResponse_ResourceDescription) validate(pattern []patternTuple) error {
	seenParams := stringset.New(len(pattern))
	for _, t := range pattern {
		if _, ok := d.PatternParameters[t.parameter]; !ok {
			return errors.Reason("parameter %q is defined in pattern, but not in pattern_params", t.parameter).Err()
		}
		seenParams.Add(t.parameter)
	}
	for p := range d.PatternParameters {
		if !seenParams.Has(p) {
			return errors.Reason("parameter %q is defined in pattern_params, but not in pattern", p).Err()
		}
	}

	if len(d.Roles) == 0 {
		return errors.Reason("roles are not defined").Err()
	}
	actions := stringset.New(len(d.Actions))
	for _, a := range d.Actions {
		if err := a.validate(); err != nil {
			return err
		}
		if !actions.Add(a.ActionId) {
			return errors.Reason("duplicate action %q", a.ActionId).Err()
		}
	}
	for roleID, role := range d.Roles {
		if !rgxRoleID.MatchString(roleID) {
			return errors.Reason("role %q does not match regexp %q", roleID, rgxRoleID).Err()
		}
		if err := role.validate(actions); err != nil {
			return errors.Annotate(err, "invalid role %q", roleID).Err()
		}
	}

	return nil
}
