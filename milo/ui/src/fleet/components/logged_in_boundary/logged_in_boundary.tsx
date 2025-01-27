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

import { Link } from '@mui/material';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl } from '@/common/tools/url_utils';

export const LoggedInBoundary = ({
  children,
}: {
  children: React.ReactNode;
}) => {
  const auth = useAuthState();

  if (auth.identity === ANONYMOUS_IDENTITY)
    return (
      <div css={{ margin: '8px 16px' }}>
        You must{' '}
        <Link
          href={getLoginUrl(
            location.pathname + location.search + location.hash,
          )}
          css={{ textDecoration: 'underline' }}
        >
          login
        </Link>{' '}
        to see anything useful.
      </div>
    );
  return children;
};
