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

import { Link } from '@mui/material';

import { Link as LinkData } from '@/common/models/link';

export interface MiloLinkProps {
  readonly link: LinkData;
  readonly target?: string;
}

/**
 * Renders a Link object.
 */
export function MiloLink({ link, target }: MiloLinkProps) {
  return (
    <Link href={link.url} aria-label={link.ariaLabel} target={target || ''}>
      {link.label}
    </Link>
  );
}
