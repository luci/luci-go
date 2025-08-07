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

// Package webauthn contains common WebAuthn types.
package webauthn

import (
	"encoding/base64"
	"encoding/json"
)

// A GetAssertionRequest represents a WebAuthn authenticatorGetAssertion request.
//
// https://www.w3.org/TR/webauthn-3/#sctn-op-get-assertion
type GetAssertionRequest struct {
	// Type (should always be "get")
	Type        string                  `json:"type"`
	Origin      string                  `json:"origin"`
	RequestData GetAssertionRequestData `json:"requestData"`
}

type GetAssertionRequestData struct {
	// Relying Party ID
	RPID string `json:"rpId"`
	// Challenge, URL safe base64 encoded
	Challenge        string                          `json:"challenge"`
	TimeoutMillis    int                             `json:"timeout,omitempty"`
	AllowCredentials []PublicKeyCredentialDescriptor `json:"allowCredentials,omitempty"`
	UserVerification string                          `json:"userVerification,omitempty"`
	Extensions       map[string]any                  `json:"extensions,omitempty"`
}

// A GetAssertionResponse represents a WebAuthn authenticatorGetAssertion response.
type GetAssertionResponse struct {
	// Type (should always be "getResponse")
	Type         string                   `json:"type"`
	ResponseData GetAssertionResponseData `json:"responseData"`
	Error        string                   `json:"error,omitempty"`
}

type GetAssertionResponseData struct {
	// Type (should always be "public-key")
	Type string `json:"type"`
	// Credential ID/key handle, URL safe base64 encoded
	ID                      string                         `json:"id"`
	Response                AuthenticatorAssertionResponse `json:"response"`
	AuthenticatorAttachemnt string                         `json:"authenticatorAttachment,omitempty"`
	ClientExtensionResults  json.RawMessage                `json:"clientExtensionResults,omitempty"`
}

// A PublicKeyCredentialDescriptor describes a security key credential.
//
// https://www.w3.org/TR/webauthn-3/#dictionary-credential-descriptor
type PublicKeyCredentialDescriptor struct {
	// Type (should always be "public-key")
	Type string `json:"type"`
	// Key handle, URL safe base64 encoded
	ID string `json:"id"`
	// Supported transports
	Transports []string `json:"transports,omitempty"`
}

// An AuthenticatorAssertionResponse is what it says.
//
// https://www.w3.org/TR/webauthn-3/#authenticatorassertionresponse
type AuthenticatorAssertionResponse struct {
	// Client data JSON, URL safe base64 encoded
	ClientData string `json:"clientDataJSON"`
	// Authenticator data, URL safe base64 encoded
	AuthenticatorData string `json:"authenticatorData"`
	// URL safe base64 encoded
	Signature string `json:"signature"`
	// URL safe base64 encoded
	UserHandle string `json:"userHandle,omitempty"`
}

// Base64 encoding used for many WebAuthn fields.
var Base64Encoding = base64.RawURLEncoding
