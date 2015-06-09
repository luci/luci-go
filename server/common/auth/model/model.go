// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package model

// AuthReplicationState contains state used to control Primary to Replica replicaiton.
type AuthReplicationState struct {
	// PrimaryID represents a GAE application ID of Primary.
	PrimaryID string
	// PrimaryURL represents a root URL of Primary, i.e. https://<host>
	PrimaryURL string
	// AuthDBRev represents a revision of AuthDB.
	AuthDBRev int64
	// ModifiedTimestamp represents a time when AuthDBRev was created (by Primary clock).
	ModifiedTimestamp time.Time
}

// AuthGlobalConfig is a root entity for auth models.
type AuthGlobalConfig struct {
	// OAuthClientID represents OAuth2 client id.
	OAuthClientID string
	// OAuthClientSecret represents OAuth2 client secret.
	OAuthClientSecret string
	// OAuthAdditionalClientIDs is client IDs allowed to access the services.
	OAuthAdditionalClientIDs []string
}

// AuthGroup is a group of identities.
type AuthGroup struct {
	// Members represents a list of members that are explicitly in this group.
	Members []string
	// Globs is a list of identity-glob expressions.
	// e.g. user:*@example.com
	Globs []string
	// Nested is a list of nested group names.
	Nested []string

	// Description is a human readable description.
	Description string

	// CreatedTimestamp represents when the group was created.
	CreatedTimestamp time.Time
	// CreatedBy represents who created the group.
	CreatedBy string

	// ModifiedTimestamp represents when the group was modified.
	ModifiedTimestamp time.Time
	// ModifiedBy represents who modified the group.
	ModifiedBy string
}

// AuthIPWhitelist is a named set of whitelisted IPv4 and IPv6 subnets.
type AuthIPWhitelist struct {
	// Subnets is the list of subnets.
	Subnets []string

	// Description is a human readable description.
	Description string

	// CreatedTimestamp represents when the list was created.
	CreatedTimestamp time.Time
	// CreatedTimestamp represents who created the list.
	CreatedBy string

	// ModifiedTimestamp represents when the list was modified.
	ModifiedTimestamp time.Time
	// ModifiedTimestamp represents who modified the list.
	ModifiedBy string
}

// Assignment is a internal data structure used in AuthIPWhitelistAssignments.
type Assignment struct {
	// Identity is a name to limit by IP whitelist.
	Identity string
	// IPWhitelist is a name of IP whitelist to use.
	IPWhitelist string
	// Comment represents the reason why the assignment was created.
	Comment string

	// CreatedTimestamp represents when the assignment was created.
	CreatedTimestamp time.Time
	// CreatedTimestamp represents who created the assignment.
	CreatedBy string
}

// AuthIPWhitelistAssignments is a singleton entity with "identity -> AUthIPWhitelist to use" mapping.
type AuthIPWhitelistAssignments struct {
	// Assignments holds all the assignments.
	Assignments []Assignment
}
