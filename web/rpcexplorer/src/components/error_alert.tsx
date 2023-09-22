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

import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';

import { RPCError, StatusCode } from '../data/prpc';

export interface Props {
  title?: string;
  error: Error;
}

export const ErrorAlert = ({ title, error }: Props) => {
  let code = '';
  let http = 0;
  let text = '';
  if (error instanceof RPCError) {
    code = StatusCode[error.code];
    http = error.http;
    text = error.text || 'Unknown error';
  } else {
    text = error.message || 'Unknown error';
  }
  return (
    <Alert severity='error'>
      {title && <AlertTitle>{title}</AlertTitle>}
      {code && <><b>{code}</b> (HTTP status {http}): </>}
      {text}
    </Alert>
  );
};


export default ErrorAlert;
