// Copyright 2023 The LUCI Authors.
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

import { Box, SxProps, Theme } from '@mui/material';
import { useMemo } from 'react';

import { sanitizeHTML } from '@/common/tools/sanitize_html';

export interface SanitizedHtmlProps {
  /**
   * The HTML TO BE SANITIZED and rendered.
   */
  readonly html: string;
  readonly sx?: SxProps<Theme>;
  readonly className?: string;
}

/**
 * Sanitize & render the HTML into a <Box />.
 *
 * Note that this should be used even when a trusted type policy is used. This
 * adds another layer of defense in case the trusted type policy is not active.
 * This may happen when
 * 1. The trusted type policy is misconfigured.
 * 2. A proper CSP context is not set via the HTTP header or HTML meta tag.
 *
 * It also makes components under unit test behave closer production. As trusted
 * type policy and CSP are not typically used in unit tests.
 */
export function SanitizedHtml({ html, sx, className }: SanitizedHtmlProps) {
  // `sanitizedHtml` must be a `TrustedHTML` not a `string` to ensure the HTML
  // doesn't get sanitized again by the default trusted type policy.
  //
  // Declare the type explicitly to detect breakage in the future.
  const sanitizedHtml: TrustedHTML = useMemo(
    () => sanitizeHTML(html, { RETURN_TRUSTED_TYPE: true }),
    [html],
  );

  return (
    <Box
      sx={sx}
      className={className}
      // We've sanitized the HTML.
      // eslint-disable-next-line react/no-danger
      dangerouslySetInnerHTML={{
        __html: sanitizedHtml,
      }}
    />
  );
}
