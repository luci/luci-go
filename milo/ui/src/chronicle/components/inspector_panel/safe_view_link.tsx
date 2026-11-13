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

import { Link } from '@mui/material';

function getSafeViewUrl(url: string): string | null {
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return null;
  }

  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') {
    return null;
  }

  return parsed.href;
}

/**
 * Renders a TurboCI view URL as a link only if it is an absolute http(s) URL;
 * otherwise renders it as plain text.
 */
export function SafeViewLink({ url }: { url: string }) {
  const safeUrl = getSafeViewUrl(url);
  if (!safeUrl) {
    return <>{url}</>;
  }
  return (
    <Link href={safeUrl} target="_blank" rel="noopener">
      {url}
    </Link>
  );
}
