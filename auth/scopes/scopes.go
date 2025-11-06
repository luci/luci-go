// Copyright 2025 The LUCI Authors.
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

// Package scopes defines values of some know OAuth scopes.
//
// When possible, prefer to use scope sets returned by functions in this
// package. Each unique combination of requested scopes requires the user to go
// through a login process. Consistently using the same set of scopes across
// different tools reduces the total number of logins required.
package scopes

// TODO: Get rid of IAM in favor of CloudScopeSet().

const (
	CloudPlatform = "https://www.googleapis.com/auth/cloud-platform"
	Email         = "https://www.googleapis.com/auth/userinfo.email"
	Firebase      = "https://www.googleapis.com/auth/firebase"
	Gerrit        = "https://www.googleapis.com/auth/gerritcodereview"
	IAM           = "https://www.googleapis.com/auth/iam"
	ReAuth        = "https://www.googleapis.com/auth/accounts.reauth"
	BCIDVerify    = "https://www.googleapis.com/auth/bcid_verify"
)

// ContextScopeSet returns a set of scopes of tokens returned by local LUCI auth
// service in a context of a LUCI build or equivalent `luci-auth context`.
func ContextScopeSet() []string {
	return []string{
		CloudPlatform,
		Email,
		Firebase,
		Gerrit,
	}
}

// CloudScopeSet returns a list of scopes to use in tools that talk exclusively
// to GCP.
func CloudScopeSet() []string {
	return []string{
		CloudPlatform,
		Email,
	}
}

// GerritScopeSet returns a list of scopes to use in tools that talk exclusively
// to Gerrit.
//
// Note: This can be the same as CloudScopeSet(), since cloud-platform scope
// coverts Gerrit API usage. The only concern  is that tools that interact with
// Gerrit will suddenly ask users to consent to access their Cloud data.
func GerritScopeSet() []string {
	return []string{
		Email,
		Gerrit,
	}
}
