// Copyright 2021 The LUCI Authors.
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

// Package mailer can be used to send mails through a mailer service.
//
// Install it as a server module and then use Send(...) API.
//
// Using "gae" backend requires running on GAE and having
// "app_engine_apis: true" in the module YAML.
// See https://cloud.google.com/appengine/docs/standard/go/services/access.
package mailer

import (
	"context"
)

var mailerCtxKey = "go.chromium.org/luci/server/mailer.Mailer"

// Mail represents an email message.
//
// Addresses may be of any form permitted by RFC 822. At least one of To, Cc,
// or Bcc must be non-empty. At least one of TextBody or HTMLBody must be
// non-empty.
type Mail struct {
	// Sender is put into "From" email header field.
	Sender string
	// ReplyTo is put into "Reply-To" email header field if not empty.
	ReplyTo string
	// To is put into "To" email header field.
	To []string
	// Cc is put into "Cc" email header field.
	Cc []string
	// Bcc is put into "Bcc" email header field.
	Bcc []string
	// Subject is put into "Subject" email header field.
	Subject string
	// TextBody is a plaintext body of the email message.
	TextBody string
	// HTMLBody is an HTML body of the email message.
	HTMLBody string
}

// Mailer lives in the context and knows how to send Messages.
//
// Must return gRPC errors.
type Mailer func(ctx context.Context, msg *Mail) error

// Use replaces the mailer in the context.
//
// Useful in tests to mock the real mailer.
func Use(ctx context.Context, m Mailer) context.Context {
	return context.WithValue(ctx, &mailerCtxKey, m)
}

// Send sends the given message through a mailer client in the context.
//
// Panics if the context doesn't have a mailer. Returns gRPC errors.
func Send(ctx context.Context, msg *Mail) error {
	m, _ := ctx.Value(&mailerCtxKey).(Mailer)
	if m == nil {
		panic("no mailer in the context")
	}
	return m(ctx, msg)
}
