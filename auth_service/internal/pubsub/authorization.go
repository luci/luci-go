// Copyright 2024 The LUCI Authors.
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

package pubsub

import (
	"context"
	"fmt"
	"strings"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/auth"
)

const (
	// The IAM role required for PubSub subscribing.
	subscriberRole = "roles/pubsub.subscriber"

	// Prefixes for account identifiers, to be used with email addresses.
	serviceAccountPrefix = "serviceAccount:"
	userPrefix           = "user:"

	// Prefix for a deleted account,
	// e.g. "deleted:serviceAccount:project-name@example.serviceaccount.com"
	deletedPrefix = "deleted:"
)

// IsAuthorizedSubscriber returns whether the account is authorized to
// subscribe to Pubsub notifications of AuthDB changes.
func IsAuthorizedSubscriber(ctx context.Context, email string) (authorized bool, retErr error) {
	client, err := newClient(ctx)
	if err != nil {
		return false, errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return false, err
	}

	return policy.HasRole(emailToIAMIdentity(email), subscriberRole), nil
}

// AuthorizeSubscriber authorizes the account to subscribe to Pubsub
// notifications of AuthDB changes.
//
// Note this does not actually create the subscription, but rather,
// makes the account eligible to subscribe.
func AuthorizeSubscriber(ctx context.Context, email string) (retErr error) {
	client, err := newClient(ctx)
	if err != nil {
		return errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting IAM policy for PubSub topic").Err()
	}

	identity := emailToIAMIdentity(email)
	if policy.HasRole(identity, subscriberRole) {
		// Already authorized to subscribe.
		return nil
	}

	// Grant authorization to subscribe.
	logging.Infof(ctx, "granting PubSub authorization for %s", email)
	policy.Add(identity, subscriberRole)
	if err := client.SetIAMPolicy(ctx, policy); err != nil {
		return errors.Annotate(err, "failed to authorize %s", identity).Err()
	}

	return nil
}

// DeauthorizeSubscriber revokes the subscribing authorization for the
// account to Pubsub notifications of AuthDB changes (i.e. making it
// ineligible to subscribe).
func DeauthorizeSubscriber(ctx context.Context, email string) (retErr error) {
	client, err := newClient(ctx)
	if err != nil {
		return errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting IAM policy for PubSub topic").Err()
	}

	id := emailToIAMIdentity(email)
	if !policy.HasRole(id, subscriberRole) {
		// Already unauthorized to subscribe.
		return nil
	}

	// Revoke authorization to subscribe.
	logging.Infof(ctx, "revoking PubSub authorization for %s", email)
	policy.Remove(id, subscriberRole)
	if err := client.SetIAMPolicy(ctx, policy); err != nil {
		return errors.Annotate(err, "failed to deauthorize %s", id).Err()
	}

	return nil
}

// RevokeStaleAuthorization revokes the subscribing authorization for
// all accounts that are no longer in the given trusted group.
func RevokeStaleAuthorization(ctx context.Context, trustedGroup string) (retErr error) {
	s := auth.GetState(ctx)
	if s == nil {
		return fmt.Errorf("error getting AuthDB")
	}
	authDB := s.DB()

	client, err := newClient(ctx)
	if err != nil {
		return errors.Annotate(err, "error creating Pubsub client").Err()
	}
	defer func() {
		err := client.Close()
		if retErr == nil {
			retErr = err
		}
	}()

	policy, err := client.GetIAMPolicy(ctx)
	if err != nil {
		return errors.Annotate(err, "error getting IAM policy for PubSub topic").Err()
	}

	updated := false
	allAuthorized := policy.Members(subscriberRole)
	for _, iamIdentity := range allAuthorized {
		if strings.HasPrefix(iamIdentity, deletedPrefix) {
			logging.Warningf(ctx,
				"detected deleted account %q - revoking subscribing authorization",
				iamIdentity)
			policy.Remove(iamIdentity, subscriberRole)
			updated = true
			continue
		}

		email, err := iamIdentityToEmail(iamIdentity)
		if err != nil {
			// Non-fatal - just log the error.
			logging.Errorf(ctx, err.Error())
			continue
		}
		authIdentity, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, email))
		if err != nil {
			// Non-fatal - just log the error.
			logging.Errorf(ctx, err.Error())
			continue
		}

		trusted, err := authDB.IsMember(ctx, authIdentity, []string{trustedGroup})
		if err != nil {
			return errors.Annotate(err, "error checking %s membership for %s",
				trustedGroup, authIdentity).Err()
		}
		if !trusted {
			logging.Warningf(ctx, "revoking subscribing authorization for %s", iamIdentity)
			policy.Remove(iamIdentity, subscriberRole)
			updated = true
		}
	}

	if updated {
		if err := client.SetIAMPolicy(ctx, policy); err != nil {
			return errors.Annotate(err, "failed to revoke stale authorizations").Err()
		}
	}

	return nil
}

func emailToIAMIdentity(email string) string {
	if strings.HasSuffix(email, ".gserviceaccount.com") {
		return serviceAccountPrefix + email
	}
	return userPrefix + email
}

func iamIdentityToEmail(id string) (string, error) {
	switch {
	case strings.HasPrefix(id, userPrefix):
		return strings.TrimPrefix(id, userPrefix), nil
	case strings.HasPrefix(id, serviceAccountPrefix):
		return strings.TrimPrefix(id, serviceAccountPrefix), nil
	default:
		return "", fmt.Errorf("failed to extract email from IAM identity %s", id)
	}
}
