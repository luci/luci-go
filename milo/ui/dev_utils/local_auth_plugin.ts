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

import { exec } from 'node:child_process';
import { promisify } from 'node:util';

import { jwtDecode } from 'jwt-decode';
import { PluginOption } from 'vite';

import { AuthState } from '../src/common/api/auth_state';

const OAUTH_SCOPES = [
  'profile',
  // This is required for `luci-auth token -use-id-token`.
  // `email` does not work.
  'https://www.googleapis.com/auth/userinfo.email',
  'https://www.googleapis.com/auth/gerritcodereview',
  'https://www.googleapis.com/auth/buganizer',
].join(' ');

const execCLI = promisify(exec);

interface TokenJSON {
  readonly token: string;
  readonly expiry: number;
}

interface ProfileJSON {
  readonly picture: string;
  readonly email: string;
}

async function getAuthStateFromLUCIAuth(): Promise<AuthState> {
  const cmdTemplate = `luci-auth token -json-output - -scopes "${OAUTH_SCOPES}"`;

  const { stdout: accessTokenJSON } = await execCLI(cmdTemplate);
  const accessToken = JSON.parse(accessTokenJSON) as TokenJSON;
  const { stdout: idTokenJSON } = await execCLI(cmdTemplate + ' -use-id-token');
  const idToken = JSON.parse(idTokenJSON) as TokenJSON;
  const profile = jwtDecode<ProfileJSON>(idToken.token);
  return {
    identity: `user:${profile.email}`,
    picture: profile.picture,
    email: profile.email,
    accessToken: accessToken.token,
    accessTokenExpiry: accessToken.expiry,
    idToken: idToken.token,
    idTokenExpiry: idToken.expiry,
  };
}

/**
 * A vite plugin that enables a `luci-auth` based login flow during local
 * development.
 */
export function localAuthPlugin(): PluginOption {
  return {
    name: 'luci-ui-local-auth',
    configureServer: (server) => {
      // Instructs developers to sign in with the luci-auth flow.
      server.middlewares.use(async (req, res, next) => {
        const url = new URL(req.url!, 'http://placeholder.com');
        if (url.pathname !== '/auth/openid/login') {
          return next();
        }

        const params = new URLSearchParams({
          redirect: url.searchParams.get('r') || '/',
          scopes: OAUTH_SCOPES,
        });
        res.statusCode = 302;
        res.setHeader(
          'Location',
          `/ui/local-login-instruction?${params.toString()}`,
        );
        res.end();
      });

      // Return a auth state object if a valid luci-auth session exists.
      server.middlewares.use(async (req, res, next) => {
        if (req.url !== '/auth/openid/state') {
          return next();
        }

        let authState: AuthState;
        try {
          authState = await getAuthStateFromLUCIAuth();
        } catch (_e) {
          return next();
        }

        res.setHeader('content-type', 'application/json');
        res.end(JSON.stringify(authState));
      });

      // Logout the luci-auth session.
      server.middlewares.use(async (req, res, next) => {
        const url = new URL(req.url!, 'http://placeholder.com');
        if (url.pathname !== '/auth/openid/logout') {
          return next();
        }

        await execCLI(`luci-auth logout -scopes "${OAUTH_SCOPES}"`);

        res.setHeader('Location', url.searchParams.get('r') || '/');
        res.statusCode = 302;
        res.end();
      });
    },
  };
}
