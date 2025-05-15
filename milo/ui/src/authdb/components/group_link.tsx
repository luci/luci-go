// Copyright 2025 The LUCI Authors.
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
import Typography from '@mui/material/Typography';
import { Link as RouterLink } from 'react-router';

import { getURLPathFromAuthGroup } from '@/common/tools/url_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';

interface GroupLinkProps {
  name: string;
}

export function GroupLink({ name }: GroupLinkProps) {
  const [searchParams] = useSyncedSearchParams();

  return (
    <Link
      component={RouterLink}
      to={getURLPathFromAuthGroup(name, searchParams.get('tab'))}
      data-testid={`${name}-link`}
    >
      <Typography variant="body2">{name}</Typography>
    </Link>
  );
}
