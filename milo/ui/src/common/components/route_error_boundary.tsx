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

import styled from '@emotion/styled';
import { Alert, AlertTitle, Link } from '@mui/material';
import { useRouteError } from 'react-router-dom';

import { genFeedbackUrl } from '@/common/tools/utils';

const ErrorDisplay = styled.pre({
  whiteSpace: 'pre-wrap',
  overflowWrap: 'break-word',
});

export function RouteErrorBoundary() {
  const error = useRouteError() as Error;

  return (
    <Alert severity="error">
      <AlertTitle>Error</AlertTitle>
      <ErrorDisplay>{error.message}</ErrorDisplay>
      <Link
        href={genFeedbackUrl(error.message, error.stack)}
        target="_blank"
        rel="noopener"
      >
        File a bug
      </Link>
    </Alert>
  );
}
