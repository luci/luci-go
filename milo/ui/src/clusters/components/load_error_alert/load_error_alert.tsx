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

import { GrpcError, RpcCode } from '@chopsui/prpc-client';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Link from '@mui/material/Link';
import { useRef } from 'react';
import { useLocation } from 'react-router-dom';

import { ANONYMOUS_IDENTITY } from '@/common/api/auth_state';
import { useAuthState } from '@/common/components/auth_state_provider';
import { getLoginUrl } from '@/common/tools/url_utils';

interface Props {
  // A human readable description of the entity being loaded,
  // in lower case. For example, "failures" or "clusters".
  entityName: string;
  // The error obtained loading the entity.
  error: Error;
}

const LoadErrorAlert = ({ entityName, error }: Props) => {
  const authState = useAuthState();
  const initialIdentity = useRef(authState.identity);
  const location = useLocation();

  let message = error.message;
  if (error instanceof GrpcError) {
    message = error.codeName + ': ' + error.description;
  }
  const isPermissionDenied =
    error instanceof GrpcError &&
    (error.code === RpcCode.PERMISSION_DENIED ||
      error.code === RpcCode.UNAUTHENTICATED);
  return (
    <Alert severity="error" sx={{ mb: 2 }}>
      <AlertTitle>Failed to load {entityName}</AlertTitle>
      {isPermissionDenied && initialIdentity.current === ANONYMOUS_IDENTITY ? (
        <>
          Please{' '}
          <Link
            data-testid="error_login_link"
            href={getLoginUrl(
              location.pathname + location.search + location.hash,
            )}
          >
            log in
          </Link>{' '}
          to view this information.
        </>
      ) : (
        message
      )}
    </Alert>
  );
};

export default LoadErrorAlert;
