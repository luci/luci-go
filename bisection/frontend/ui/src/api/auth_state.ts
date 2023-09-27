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

let authState: AuthState | null = null;

export interface AuthState {
  identity: string;
  email: string;
  picture: string;
  accessToken: string;
  idToken: string;
  // Expiration time (unix timestamp) of the access token.
  // If zero/undefined, the access token does not expire.
  accessTokenExpiry: number;
  idTokenExpiry: number;
}

async function queryAuthState(): Promise<AuthState> {
  const res = await fetch('/api/authState');
  if (!res.ok) {
    throw new Error('failed to get authState:\n' + (await res.text()));
  }
  return res.json();
}

/**
 * obtainAuthState obtains a current auth state, for interacting
 * with pRPC APIs.
 * @return the current auth state.
 */
export async function obtainAuthState(): Promise<AuthState> {
  if (
    authState != null &&
    authState.accessTokenExpiry * 1000 > Date.now() + 5000 &&
    authState.idTokenExpiry * 1000 > Date.now() + 5000
  ) {
    // Auth state still has >=5 seconds of validity for both tokens.
    return authState;
  }

  // Refresh the auth state.
  const response = await queryAuthState();
  authState = response;
  return authState;
}
