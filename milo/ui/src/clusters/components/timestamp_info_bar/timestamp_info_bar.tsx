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

import './styles.css';

import Grid from '@mui/material/Grid2';
import Link from '@mui/material/Link';
import { DateTime } from 'luxon';

import { RelativeTimestamp } from '@/common/components/relative_timestamp';
import { displayApproxDuration } from '@/common/tools/time_utils';

interface Props {
  createUsername: string | undefined;
  createTime: string | undefined;
  updateUsername: string | undefined;
  updateTime: string | undefined;
}

interface FormattedUsernameProps {
  username: string | undefined;
}

const FormattedUsername = ({ username }: FormattedUsernameProps) => {
  if (!username) {
    return <></>;
  }
  if (username === 'system') {
    return <>LUCI Analysis</>;
  } else if (username.endsWith('@google.com')) {
    const ldap = username.substring(0, username.length - '@google.com'.length);
    return (
      <Link target="_blank" href={`http://who/${ldap}`}>
        {ldap}
      </Link>
    );
  } else {
    return <>{username}</>;
  }
};

const TimestampInfoBar = ({
  createUsername,
  createTime,
  updateUsername,
  updateTime,
}: Props) => {
  return (
    <Grid container>
      <Grid>
        <small
          data-testid="timestamp-info-bar-create"
          className="timestamp-text"
        >
          Created
          {createUsername && (
            <> by {<FormattedUsername username={createUsername} />}</>
          )}{' '}
          <RelativeTimestamp
            formatFn={displayApproxDuration}
            timestamp={DateTime.fromISO(createTime || '')}
          />
          .
        </small>
        <small
          data-testid="timestamp-info-bar-update"
          className="timestamp-text"
        >
          {' '}
          Last modified
          {updateUsername && (
            <> by {<FormattedUsername username={updateUsername} />}</>
          )}{' '}
          <RelativeTimestamp
            formatFn={displayApproxDuration}
            timestamp={DateTime.fromISO(updateTime || '')}
          />
          .
        </small>
      </Grid>
    </Grid>
  );
};

export default TimestampInfoBar;
