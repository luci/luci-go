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


// OAuthError can be raised by accessToken().
export class OAuthError extends Error {
  // If true, the user explicitly canceled the login flow.
  readonly cancelled: boolean;

  constructor(msg: string, cancelled = false) {
    super(msg);
    Object.setPrototypeOf(this, OAuthError.prototype);
    this.cancelled = cancelled;
  }
}


// Account represents a logged in user account.
export interface Account {
  email: string;
  picture?: string;
}


// Knows how to get and refresh OAuth access tokens.
export interface TokenClient {
  // The state of the session, if any. Affects Login/Logout UI.
  sessionState: Account | 'loggedout' | 'stateless';
  // URL to send the browser to to go through the login flow.
  loginUrl: string | undefined;
  // URL to send the browser to to go through the logout flow.
  logoutUrl: string | undefined;
  // Returns a fresh OAuth access token to use in a request.
  accessToken: () => Promise<string>;
}


// A TokenClient that uses google.accounts.oauth2 to get and refresh tokens.
export class OAuthClient {
  // This client doesn't use sessions.
  readonly sessionState = 'stateless';
  // Login is not supported.
  readonly loginUrl = undefined;
  // Logout is not supported.
  readonly logoutUrl = undefined;

  private tokenClient: google.accounts.oauth2.TokenClient;
  private cachedToken: { token: string; expiry: number; } | null = null;
  private waiters: {
    resolve: (token: string) => void,
    reject: (error: Error) => void,
  }[] = [];

  constructor(clientId: string, scopes?: string[]) {
    this.tokenClient = google.accounts.oauth2.initTokenClient({
      client_id: clientId,
      scope: scopes?.join(' ') || 'https://www.googleapis.com/auth/userinfo.email',
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
      this.rejectAllWaiters(
          `${response.error}: ${response.error_description}`,
          false,
      );
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
    this.rejectAllWaiters(
        `${error.type}: ${error.message}`,
        error.type == 'popup_closed',
    );
  }

  private rejectAllWaiters(error: string, cancelled: boolean) {
    const waiters = this.waiters;
    this.waiters = [];
    for (const waiter of waiters) {
      waiter.reject(new OAuthError(error, cancelled));
    }
  }
}


// Response of the auth state endpoint, see StateEndpointResponse in auth.go.
interface StateEndpointResponse {
  identity?: string;
  email?: string;
  picture?: string;
  accessToken?: string;
  accessTokenExpiresIn?: number;
}


// A TokenClient that uses the server's auth state endpoint.
export class StateEndpointClient {
  // Either logged in or not.
  sessionState: Account | 'loggedout' = 'loggedout';
  // URL to send the browser to to go through the login flow.
  loginUrl: string;
  // URL to send the browser to to go through the logout flow.
  logoutUrl: string;

  private stateUrl: string;
  private cachedToken: { token: string; expiry: number; } | null = null;

  constructor(stateUrl: string, loginUrl: string, logoutUrl: string) {
    this.stateUrl = stateUrl;
    this.loginUrl = loginUrl;
    this.logoutUrl = logoutUrl;
  }

  async fetchState() {
    const response = await fetch(this.stateUrl, {
      credentials: 'same-origin',
      headers: { 'Accept': 'application/json' },
    });
    const state = await response.json() as StateEndpointResponse;
    if (state.identity == 'anonymous:anonymous') {
      this.sessionState = 'loggedout';
      this.cachedToken = null;
      return;
    }
    if (!state.email) {
      throw new OAuthError('Missing email in the auth state response');
    }
    if (!state.accessToken) {
      throw new OAuthError('Missing token in the auth state response');
    }
    if (!state.accessTokenExpiresIn) {
      throw new OAuthError('Missing expiry in the auth state response');
    }
    this.sessionState = {
      email: state.email,
      picture: state.picture,
    };
    // Remove 60s from the lifetime to make sure we never try to use a token
    // which is very close to expiration.
    this.cachedToken = {
      token: state.accessToken,
      expiry: Date.now() + (state.accessTokenExpiresIn - 60) * 1000,
    };
  }

  // Returns an access token, refreshing it if necessary.
  //
  // Raises an OAuthError if the user is not logged in.
  async accessToken(): Promise<string> {
    // Need a session to get a token.
    if (this.sessionState == 'loggedout') {
      throw new OAuthError('Not logged in');
    }
    // If already have a fresh token, just use it.
    if (this.cachedToken && Date.now() < this.cachedToken.expiry) {
      return this.cachedToken.token;
    }
    // Need to refresh. This may raise OAuthError or change sessionState.
    await this.fetchState();
    // This will be nil if the session disappeared.
    if (!this.cachedToken) {
      throw new OAuthError('Not logged in');
    }
    return this.cachedToken.token;
  }
}


// Loads the OAuth config from the server and initializes the token client.
export const loadTokenClient = async (): Promise<TokenClient> => {
  const response = await fetch('/rpcexplorer/config', {
    credentials: 'omit',
    headers: { 'Accept': 'application/json' },
  });

  // See rpcexplorer.go for the Go side.
  interface Config {
    loginUrl?: string;
    logoutUrl?: string;
    authStateUrl?: string;
    clientId?: string;
    scopes?: string[];
  }
  const cfg = await response.json() as Config;

  // If authStateUrl is set, so should be loginUrl and logoutUrl. Use a client
  // that knows how to grab tokens from the state URL.
  if (cfg.authStateUrl) {
    if (!cfg.loginUrl) {
      throw new Error('loginUrl in the config is empty');
    }
    if (!cfg.logoutUrl) {
      throw new Error('logoutUrl in the config is empty');
    }
    const client = new StateEndpointClient(
        cfg.authStateUrl, cfg.loginUrl, cfg.logoutUrl,
    );
    // Check if there's a session already.
    await client.fetchState();
    return client;
  }

  // A client that uses google.accounts.oauth2 Javascript library.
  if (!cfg.clientId) {
    throw new Error('clientId in the config is empty');
  }
  return new OAuthClient(cfg.clientId, cfg.scopes);
};
