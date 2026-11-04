// Copyright 2026 The LUCI Authors.
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

export interface SemaphoreMessage {
  readonly name: string;
  readonly longMessage?: string;
  readonly shortMessage?: string;
  readonly urgency?: 'FAIL' | 'WARN' | 'INFO';
}

// Ported from `getUrgencyRank` and `getUrgencyCssClass` in
// http://shortn/_XFfU9K682V.
function getUrgencyRank(urgency?: string): number {
  switch (urgency) {
    case 'FAIL':
      return 0;
    case 'INFO':
      return 2;
    case 'WARN':
    default:
      return 1;
  }
}

async function listGlobalMessages(
  host: string,
  apiKey: string,
): Promise<SemaphoreMessage[]> {
  const response = await fetch(
    `https://${host}/v4/messages?enabled=true&scopes=global&key=${encodeURIComponent(apiKey)}`,
    { credentials: 'include' },
  );
  if (!response.ok) {
    throw new Error(`Semaphore API returned HTTP ${response.status}`);
  }
  const { messages = [] } = (await response.json()) as {
    messages?: SemaphoreMessage[];
  };
  return [...messages].sort(
    (a, b) => getUrgencyRank(a.urgency) - getUrgencyRank(b.urgency),
  );
}

// Cookie refresh of `UberproxyFetchPromiseRequestService` ported from
// http://shortn/_mV4Hypkyqc (`isPossibleFetchCorsError`,
// `refreshApiUberproxyCookies` and the first retry in `makeRequest`), without
// its final fallback of prompting the user to sign in to ÜberProxy. The
// Semaphore ÜberProxy service only lets requests without an Origin header, such
// as a no-cors fetch, through to `/js/empty.js`, so that path is fetched instead
// of `/expected_404_for_cookie_refresh.txt`.
function isPossibleFetchCorsError(error: unknown) {
  // CORS errors are VERY opaque
  return error instanceof TypeError;
}

function refreshApiUberproxyCookies(host: string): Promise<unknown> {
  // We just need to load a page under the api, it's ok if it's a 404.
  // Because mode=no-cors the response is opaque so we can't get the response
  // data (and we don't care because we are just ensuring the cookies get set
  // for future requests).
  return fetch(`https://${host}/js/empty.js`, {
    method: 'GET',
    credentials: 'include',
    mode: 'no-cors',
  });
}

/** Lists the enabled global Semaphore messages, most severe first. */
export async function fetchSemaphoreMessages(
  host: string,
  apiKey: string,
): Promise<SemaphoreMessage[]> {
  // try to make the request
  try {
    return await listGlobalMessages(host, apiKey);
  } catch (e: unknown) {
    if (!isPossibleFetchCorsError(e)) throw e;
  }

  // if the request fails due to CORS try refreshing the cookies on the
  // uberproxy domain. This relies on the user SSO sign in still being valid
  // so the cookie can be refreshed without a gnubby touch (e.g. they signed
  // into uberproxy recently).
  await refreshApiUberproxyCookies(host);

  // then try the request again
  return listGlobalMessages(host, apiKey);
}
