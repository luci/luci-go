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

import { redirect, type LoaderFunction } from 'react-router';

/**
 * Build a loader that redirects users to the child path while keeping the
 * search query param.
 *
 * This is typically used to redirect users to a specific sub-route by default.
 * e.g. Redirect users from `/my-page` to `my-page/default-tab`.
 */
export function redirectToChildPath(childPath: string): LoaderFunction {
  return ({ request }) => {
    const url = new URL(request.url);
    // Note that `${url.hash}` is always `''`. We only include it in the URL for
    // completeness. react-router-dom@6.23.1 does not pass the hash info to a
    // loader. This is by design. Loaders are designed to handle requests like a
    // server.
    //
    // If we really want to preserve the hash, we can read it from
    // `self.location.hash`. However, relying on such behavior will lock us into
    // a client-side based redirection approach. We will never be able to
    // replicate this redirection on the server side as URL hashes are not sent
    // to the servers.
    return redirect(`${childPath}${url.search}${url.hash}`);
  };
}
