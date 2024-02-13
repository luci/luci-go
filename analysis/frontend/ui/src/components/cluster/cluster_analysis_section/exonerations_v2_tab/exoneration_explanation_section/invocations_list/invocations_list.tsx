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

import Link from '@mui/material/Link';

import { Fragment } from 'react';
import {
  failureLink, invocationName,
} from '@/tools/urlHandling/links';

interface Props {
  testId: string;
  invocations: readonly string[];
}

const InvocationList = ({
  testId,
  invocations,
}: Props) => {
  return (
    <>
      {invocations.map((invocationId: string, j: number) => {
        return (
          <Fragment key={invocationId}>
            {(j > 0) && ', '}
            <Link
              href={failureLink(invocationId, testId)}
              target="_blank"
            >
              {invocationName(invocationId)}
            </Link>
          </Fragment>
        );
      })}
    </>
  );
};

export default InvocationList;
