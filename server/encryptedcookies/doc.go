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

// Package encryptedcookies implements authentication using encrypted cookies.
//
// A session cookie contains a session ID and a per-session encryption key.
// The session cookie is itself encrypted (using an AEAD scheme) with the
// server-global primary encryption key. The session ID points to a session
// entity in a database that contains information about the user as well as
// various OAuth2 tokens established during the login flow when the session
// was created. Tokens are encrypted (again using an AEAD scheme) with the
// per-session encryption key from the cookie.
//
// One of the stored tokens is an OAuth2 refresh token. Periodically, during the
// session validation process, it is used to refresh other stored tokens
// (in particular an OAuth2 access token and an OpenID Connect ID token). This
// procedure fails if the user revoked access to their accounts via the ID
// provider or if the user's account is not longer active (from the ID
// provider's point of view). That way the state of the session is tied to the
// state of the user account at the ID provider which fully offloads user
// accounts management to the ID provider.
//
// # Configuration
//
// To use this module you'll need to create an encryption key and to register
// an OAuth2 client with the ID provider. Instructions below assume you are
// using the Google Accounts ID provider and the created encryption key will be
// used as the server primary encryption key (i.e. it will be used to encrypt
// not only cookies but also any other secrets that the server may wish to
// encrypt).
//
// Start by creating two Google Secret Manager secrets (with no values) named
// `tink-aead-primary` and `oauth-client-secret` in the cloud project that
// the server is running in (referred to as `<cloud-project>` below). Make sure
// the server account has "Secret Manager Secret Accessor" role in them. These
// steps are usually done using Terraform.
//
// Initialize `tink-aead-primary` key by using
// https://go.chromium.org/luci/server/cmd/secret-tool tool:
//
//	cd server/cmd/secret-tool
//	go run main.go create sm://<cloud-project>/tink-aead-primary -secret-type tink-aes256-gcm
//
// This secret now contains a serialized Tink keyset with the primary encryption
// key. If necessary it can be rotated using the same `secret-tool` tool:
//
//	cd server/cmd/secret-tool
//	go run main.go rotation-begin sm://<cloud-project>/tink-aead-primary
//	# wait several hours for the new key to propagate into all caches
//	# confirm by looking at /chrome/infra/secrets/gsm/version metric
//	go run main.go rotation-end sm://<cloud-project>/tink-aead-primary
//
// This will add a new active key to the keyset. It will be used to encrypt
// new cookies, but the old key will still be recognized when decrypting
// existing cookies.
//
// Next, create a new OAuth2 client ID that will represent your server. Follow
// instructions on https://support.google.com/cloud/answer/6158849?hl=en and
// pick the application type "Web application". Add an authorized redirect URI
// equal to "https://<your-server-host>/auth/openid/callback". Do not add any
// authorized JavaScript origins.
//
// After creating the OAuth2 client, note the client ID (usually looks like
// "<number>-<gibberish>.apps.googleusercontent.com") and the client secret
// (just a random looking string). Put the value of the secret into a new
// `oauth-client-secret` Google Secret Manager secret using the `secret-tool`:
//
//	cd server/cmd/secret-tool
//	go run main.go create sm://<cloud-project>/oauth-client-secret -secret-type password
//	# paste the client secret
//
// All prerequisites are done. Pass the following flags to the server binary to
// instruct it to use the generated secrets and the OAuth2 client:
//
//	server \
//	    ...
//	    -primary-tink-aead-key sm://tink-aead-primary \
//	    -encrypted-cookies-client-id <number>-<gibberish>.apps.googleusercontent.com \
//	    -encrypted-cookies-client-secret sm://oauth-client-secret \
//	    -encrypted-cookies-redirect-url https://<your-server-host>/auth/openid/callback
//
// Note that the value of `-encrypted-cookies-redirect-url` must match exactly
// what you specified when creating the OAuth2 client (e.g. if you used some
// custom DNS domain name there, specify it in the `-encrypted-cookies-redirect-url`
// as well).
//
// If you want to use a dedicated key set for encrypting cookies specifically,
// replace `-primary-tink-aead-key` with `-encrypted-cookies-tink-aead-key`
// (and perhaps use some different name for the secret).
//
// # Session store
//
// The module needs to know where and how to store user sessions. Link to
// a concrete implementation (e.g. Cloud Datastore) by using the following
// blank import line in the main.go:
//
//	import (
//	  ...
//	  // Store auth sessions in the datastore.
//	  _ "go.chromium.org/luci/server/encryptedcookies/session/datastore"
//	)
//
// # Inactive sessions cleanup
//
// When using Cloud Datastore as a session storage, configure a time-to-live
// policy to delete `encryptedcookies.Session` entities based on `ExpireAt`
// field. See https://cloud.google.com/datastore/docs/ttl. This step is usually
// done via Terraform.
//
// A session is considered expired if it wasn't accessed for more than 14 days.
//
// # Exposed routes
//
// The module exposes 3 routes involved in the login/logout process:
// `/auth/openid/login`, `/auth/openid/logout` and `/auth/openid/callback`. When
// configuring your load balancer (or dispatch.yaml on Appengine), make sure
// `/auth/openid/*` requests are routed appropriately.
//
// Note that cookies established by one server process can be validated by
// another, as long as they are both configured identically (i.e. all CLI flags
// mentioned above are passed to both binaries). For example, you can configure
// the load balancer to pass all `/auth/openid/*` requests to a dedicated server
// responsible for the login/logout, but then validate user cookies on another
// server.
//
// Note also that the server that only validates cookies still needs write
// access to the session store, to be able to refresh encrypted tokens stored
// there. It means if there are some caching layers in front of the datastore,
// they must be configured identically across all servers as well.
//
// # Running locally
//
// If the server is starting in the development mode (no `-prod` flag is
// passed), and the `-encrypted-cookies-client-id` flag is not set, the module
// switches to use fake cookies that have a similar semantics to the real
// encrypted cookies, but require no extra configuration. They are absolutely
// insecure and must never be used outside of local runs. They exist only to
// simplify the local development of servers that use LoginURL/LogoutURL APIs.
package encryptedcookies
