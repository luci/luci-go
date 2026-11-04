// Copyright 2018 The LUCI Authors.
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

package gerritpb

import (
	"go.chromium.org/luci/common/errors"
)

// ValidateGetChangeRequest returns an error if r is invalid.
func ValidateGetChangeRequest(r *GetChangeRequest) error {
	switch {
	case r.Number <= 0:
		return errors.New("number must be positive")
	default:
		return nil
	}
}

// ValidateSetReviewRequest returns an error if r is invalid.
func ValidateSetReviewRequest(r *SetReviewRequest) error {
	if r.Number <= 0 {
		return errors.New("number must be positive")
	}
	if err := ValidateNotifyDetails(r.GetNotifyDetails()); err != nil {
		return err
	}
	for _, att := range r.GetAddToAttentionSet() {
		if err := ValidateAttentionSetInput(att); err != nil {
			return err
		}
	}
	for _, att := range r.GetRemoveFromAttentionSet() {
		if err := ValidateAttentionSetInput(att); err != nil {
			return err
		}
	}
	return nil
}

// ValidateRevertChangeRequest returns an error if r is invalid.
func ValidateRevertChangeRequest(r *RevertChangeRequest) error {
	if r.Number <= 0 {
		return errors.New("number must be positive")
	}
	return nil
}

// ValidateNotifyDetails returns an error if n is invalid.
func ValidateNotifyDetails(n *NotifyDetails) error {
	for _, r := range n.GetRecipients() {
		if err := ValidateNotifyDetailsRecipient(r); err != nil {
			return err
		}
	}
	return nil
}

// ValidateNotifyDetailsRecipient returns an error if n is invalid.
func ValidateNotifyDetailsRecipient(n *NotifyDetails_Recipient) error {
	if n.GetRecipientType() == NotifyDetails_RECIPIENT_TYPE_UNSPECIFIED {
		return errors.New("must specify recipient type")
	}
	return nil
}

// ValidateAttentionSetRequest returns an error if r is invalid.
func ValidateAttentionSetRequest(r *AttentionSetRequest) error {
	if r.Number <= 0 {
		return errors.New("number must be positive")
	}
	if r.GetInput() == nil {
		return errors.New("input is required")
	}
	return ValidateAttentionSetInput(r.GetInput())
}

// ValidateAttentionSetInput returns an error if i is invalid.
func ValidateAttentionSetInput(i *AttentionSetInput) error {
	if i.GetUser() == "" {
		return errors.New("user is required")
	}
	if i.GetReason() == "" {
		return errors.New("reason is required")
	}
	return nil
}

// ValidateGetMetaDiffRequest returns an error if r is invalid.
func ValidateGetMetaDiffRequest(r *GetMetaDiffRequest) error {
	switch {
	case r.Number <= 0:
		return errors.New("number must be positive")
	default:
		return nil
	}
}
