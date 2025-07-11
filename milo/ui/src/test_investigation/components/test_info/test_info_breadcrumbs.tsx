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

import NavigateNextIcon from '@mui/icons-material/NavigateNext';
import { Breadcrumbs, Typography } from '@mui/material';
import Link from '@mui/material/Link';

import { CopyToClipboard } from '@/generic_libs/components/copy_to_clipboard';
import { TestIdentifier } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/common.pb';

interface TestInfoBreadcrumbsProps {
  invocation: string;
  testIdStructured: TestIdentifier | undefined;
}

export function TestInfoBreadcrumbs({
  invocation,
  testIdStructured,
}: TestInfoBreadcrumbsProps) {
  const coarseName = testIdStructured?.coarseName || undefined;
  const fineName = testIdStructured?.fineName || undefined;
  const module = testIdStructured?.moduleName || undefined;
  const invID = invocation.split('/')[1] || '';

  return (
    <Breadcrumbs
      aria-label="breadcrumb"
      separator={<NavigateNextIcon fontSize="small"></NavigateNextIcon>}
    >
      <Typography variant="caption">
        Invocation:{' '}
        {invID && (
          <Link
            target="_blank"
            href={`/ui/test-investigate/invocations/${invID}`}
          >
            {invID}
          </Link>
        )}
        <CopyToClipboard
          textToCopy={invID}
          aria-label="Copy invocation ID."
          sx={{ ml: 0.5 }}
        />
      </Typography>
      {module && (
        <Typography variant="caption">
          Module: {module}
          <CopyToClipboard
            textToCopy={module}
            aria-label="Copy module name."
            sx={{ ml: 0.5 }}
          />
        </Typography>
      )}
      {coarseName && (
        <Typography variant="caption">
          Coarse Name: {coarseName}
          <CopyToClipboard
            textToCopy={coarseName}
            aria-label="Copy module name."
            sx={{ ml: 0.5 }}
          />
        </Typography>
      )}
      {fineName && (
        <Typography variant="caption">
          Suite: {fineName}
          <CopyToClipboard
            textToCopy={fineName}
            aria-label="Copy suite name."
            sx={{ ml: 0.5 }}
          />
        </Typography>
      )}
    </Breadcrumbs>
  );
}
