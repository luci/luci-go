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

import { Link, LinkProps } from '@mui/material';

/**
 * Props for the ExternalCaptionLink component.
 */
export type ExternalCaptionLinkProps = Omit<
  LinkProps,
  'rel' | 'target' | 'variant'
>;

/**
 * A bold, caption-sized link without underline that opens in a new tab.
 * Additional `sx` styles are merged on top of the defaults.
 */
export function ExternalCaptionLink({
  sx,
  ...props
}: ExternalCaptionLinkProps) {
  return (
    <Link
      target="_blank"
      rel="noopener noreferrer"
      variant="caption"
      sx={[
        {
          fontWeight: (theme) => theme.typography.fontWeightBold,
          textDecoration: 'none',
        },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
      {...props}
    />
  );
}
