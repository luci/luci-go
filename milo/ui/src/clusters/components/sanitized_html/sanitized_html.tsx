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

import { sanitizeHTML } from './sanitize_html/sanitize_html';

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
 * @return {JSX.Element} the Box.
 */
export function SanitizedHtml({ html, sx, className }: SanitizedHtmlProps) {
  // `sanitizedHtml` must be a `TrustedHTML` not a `string` to ensure the HTML
  // doesn't get sanitized again by the default trusted type policy.
  //
  // Declare the type explicitly to detect breakage in the future.
  const sanitizedHtml: string = useMemo(() => sanitizeHTML(html), [html]);

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
