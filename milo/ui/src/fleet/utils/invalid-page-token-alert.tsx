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

import { Alert, Button } from '@mui/material';

import {
  emptyPageTokenUpdater,
  PagerContext,
} from '@/common/components/params_pager';
import { SetURLSearchParams } from '@/generic_libs/hooks/synced_search_params/context';

export const InvalidPageTokenAlert = ({
  pagerCtx,
  setSearchParams,
}: {
  setSearchParams: SetURLSearchParams;
  pagerCtx: PagerContext;
}) => (
  <Alert
    severity="error"
    action={
      <Button
        color="inherit"
        size="small"
        onClick={() => setSearchParams(emptyPageTokenUpdater(pagerCtx))}
        variant="outlined"
      >
        Reset Page
      </Button>
    }
  >
    Pagination error: The link you followed may have expired. Please reset to
    the first page.
  </Alert>
);
