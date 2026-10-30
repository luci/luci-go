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

import { AvatarProps } from '@mui/material';
import { useContext } from 'react';

import { AuthStateContext } from '@/common/components/auth_state_provider';

const GOOGLE_CORP_EMAIL_RE = /^[a-z0-9._+-]+@google\.com$/;
const MOMA_PHOTOS_BASE_URL = 'https://moma-teams-photos.corp.google.com/photos';

export const normalizeEmail = (value?: string): string =>
  (value ?? '')
    .trim()
    .replace(/^user:/i, '')
    .trim()
    .toLowerCase();

export interface UseUserPhotoOptions {
  name?: string;
  email?: string;
  id?: string;
}

export type UseUserPhotoResult = Pick<AvatarProps, 'src' | 'alt' | 'slotProps'>;

const resolveGooglePhotoUrl = (
  email: string | undefined,
  viewerEmail: string | undefined,
  viewerIdentity: string | undefined,
  viewerPicture: string | undefined,
): string | undefined => {
  const target = normalizeEmail(email);
  const viewer = normalizeEmail(viewerEmail) || normalizeEmail(viewerIdentity);
  if (!target || !GOOGLE_CORP_EMAIL_RE.test(viewer)) {
    return undefined;
  }
  const trimmedPicture = viewerPicture?.trim();
  if (
    target === viewer &&
    trimmedPicture &&
    /^https:\/\//i.test(trimmedPicture)
  ) {
    return trimmedPicture;
  }
  if (!GOOGLE_CORP_EMAIL_RE.test(target)) {
    return undefined;
  }
  return `${MOMA_PHOTOS_BASE_URL}/${encodeURIComponent(target)}?sz=96&type=preferred&no_default_image=true&force_no_redirect=true`;
};

/**
 * Resolves `<Avatar>` image props (`src`, `alt`, and `no-referrer` `slotProps`)
 * for a `@google.com` user when viewed by an authenticated `@google.com` user.
 */
export const useUserPhoto = ({
  name,
  email,
  id,
}: UseUserPhotoOptions = {}): UseUserPhotoResult => {
  const authState = useContext(AuthStateContext)?.getAuthState();
  const src = resolveGooglePhotoUrl(
    email,
    authState?.email,
    authState?.identity,
    authState?.picture,
  );
  const displayLabel = (
    name?.trim() ||
    email?.trim() ||
    id?.trim() ||
    ''
  ).replace(/^user:/i, '');

  return {
    src,
    alt: displayLabel || undefined,
    slotProps: {
      img: {
        referrerPolicy: 'no-referrer',
      },
    },
  };
};
