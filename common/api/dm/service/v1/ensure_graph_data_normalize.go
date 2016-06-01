// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dm

import (
	"fmt"

	"github.com/luci/luci-go/common/errors"
)

// Normalize returns an error iff the TemplateInstantiation is invalid.
func (t *TemplateInstantiation) Normalize() error {
	if t.Project == "" {
		return errors.New("empty project")
	}
	return t.Specifier.Normalize()
}

// Normalize returns an error iff the request is invalid.
func (r *EnsureGraphDataReq) Normalize() error {
	if err := r.Attempts.Normalize(); err != nil {
		return err
	}

	hasAttempts := false
	for _, nums := range r.Attempts.To {
		hasAttempts = true
		if len(nums.Nums) == 0 {
			return errors.New("EnsureGraphDataReq.attempts must only include valid (non-0, non-empty) attempt numbers")
		}
	}

	if len(r.TemplateQuest) != len(r.TemplateAttempt) {
		return errors.New("mismatched template_attempt v. template_quest lengths")
	}

	if len(r.TemplateQuest) > 0 {
		for i, q := range r.TemplateQuest {
			if err := q.Normalize(); err != nil {
				return fmt.Errorf("template_quests[%d]: %s", i, err)
			}
			if err := r.TemplateAttempt[i].Normalize(); err != nil {
				return fmt.Errorf("template_attempts[%d]: %s", i, err)
			}
			if len(r.TemplateAttempt[i].Nums) == 0 {
				return fmt.Errorf("template_attempts[%d]: empty attempt list", i)
			}
		}
	}

	if len(r.Quest) == 0 && !hasAttempts {
		return errors.New("EnsureGraphDataReq must have at least one of quests and attempts")
	}

	if r.Limit == nil {
		r.Limit = &EnsureGraphDataReq_Limit{}
	}
	if r.Limit.MaxDataSize == 0 {
		r.Limit.MaxDataSize = DefaultLimitMaxDataSize
	}
	if r.Limit.MaxDataSize > MaxLimitMaxDataSize {
		r.Limit.MaxDataSize = MaxLimitMaxDataSize
	}

	if r.Include == nil {
		r.Include = &EnsureGraphDataReq_Include{}
	}
	return nil
}
