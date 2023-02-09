// Copyright 2022 The LUCI Authors.
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

// Used when the server doesn't report a client ID.
const defaultOAuthClientId =
  '446450136466-e77v49thuh5dculh78gumq3oncqe28m3.apps.googleusercontent.com';


// Knows how to get and refresh OAuth access tokens.
export class OAuthClient {
  // The configured OAuth client ID.
  readonly clientId: string;

  private tokenClient: google.accounts.oauth2.TokenClient;
  private cachedToken: { token: string; expiry: number; } | null = null;
  private waiters: {
    resolve: (token: string) => void,
    reject: (error: Error) => void,
  }[] = [];

  constructor(clientId: string) {
    this.clientId = clientId;
    this.tokenClient = google.accounts.oauth2.initTokenClient({
      client_id: clientId,
      scope: 'https://www.googleapis.com/auth/userinfo.email',
      callback: (response) => this.onTokenResponse(response),
      error_callback: (error) => this.onTokenError(error),
      // This field is unknown to @types/google.accounts 0.0.4, but we need it
      // to avoid getting unexpectedly large set of scopes. Adding this field
      // requires to forcefully type-cast the struct to TokenClientConfig.
      include_granted_scopes: false,
    } as google.accounts.oauth2.TokenClientConfig);
  }

  // Returns an access token, requesting or refreshing it if necessary.
  //
  // Can take a significant amount of time, since it waits for the user to
  // click buttons to sign in etc.
  async accessToken(): Promise<string> {
    // If already have a fresh token, just use it.
    if (this.cachedToken && Date.now() < this.cachedToken.expiry) {
      return this.cachedToken.token;
    }

    // Need to refresh. Create a promise that will be resolved when the
    // refresh is done.
    const done = new Promise<string>((resolve, reject) => {
      this.waiters.push({ resolve, reject });
    });

    // If we are the first caller, actually initiate the asynchronous flow.
    if (this.waiters.length == 1) {
      this.tokenClient.requestAccessToken();
    }

    // Wait for the flow to complete.
    return await done;
  }

  private onTokenResponse(response: google.accounts.oauth2.TokenResponse) {
    if (response.error || response.error_description) {
      this.rejectAllWaiters(`${response.error}: ${response.error_description}`);
      return;
    }

    // Remove 60s from the lifetime to make sure we never try to use a token
    // which is very close to expiration.
    this.cachedToken = {
      token: response.access_token,
      expiry: Date.now() + (parseInt(response.expires_in) - 60) * 1000,
    };

    // Resolve all waiting promises successfully.
    const waiters = this.waiters;
    this.waiters = [];
    for (const waiter of waiters) {
      waiter.resolve(this.cachedToken.token);
    }
  }

  private onTokenError(error: google.accounts.oauth2.ClientConfigError) {
    this.rejectAllWaiters(`${error.type}: ${error.message}`);
  }

  private rejectAllWaiters(error: string) {
    const waiters = this.waiters;
    this.waiters = [];
    for (const waiter of waiters) {
      waiter.reject(new Error(error));
    }
  }
}


// Loads the OAuth client ID from the server and initializes Google OAuth2
// token client.
export const loadOAuthClient = async (): Promise<OAuthClient> => {
  const response = await fetch('/auth/api/v1/server/client_id', {
    credentials: 'omit',
    headers: { 'Accept': 'application/json' },
  });
  const json = await response.json();
  return new OAuthClient(json.client_id || defaultOAuthClientId);
};
