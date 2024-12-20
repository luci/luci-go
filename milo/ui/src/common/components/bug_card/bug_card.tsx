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

import AccessTimeIcon from '@mui/icons-material/AccessTime';
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined';
import LinkIcon from '@mui/icons-material/Link';
import { CircularProgress } from '@mui/material';
import { CSSProperties } from 'react';

import {
  useGetComponentQuery,
  useGetIssueQuery,
} from '@/common/hooks/gapi_query/corp_issuetracker';

interface BugCardProps {
  bugId: string;
}
export const BugCard = ({ bugId }: BugCardProps) => {
  const {
    data: bug,
    isLoading,
    error,
  } = useGetIssueQuery({ issueId: bugId }, {});
  const {
    data: component,
    isLoading: isLoadingComponent,
    error: errorComponent,
  } = useGetComponentQuery(
    { componentId: bug?.issueState.componentId || '' },
    {
      enabled: !!bug?.issueState.componentId,
    },
  );
  if (isLoading) {
    return <CircularProgress />;
  }
  if (error) {
    return <div>Error: {(error as Error)?.message}</div>;
  }
  if (!bug) {
    return <div>Error: No bug data received.</div>;
  }
  return (
    <div
      style={{
        maxWidth: '500px',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
      }}
    >
      <div style={{ ...rowCss, ...titleCss }}>{bug.issueState.title}</div>
      <div style={rowCss}>
        {isLoadingComponent ? <CircularProgress /> : null}
        {errorComponent ? (
          <div>Error: {(errorComponent as Error)?.message}</div>
        ) : null}
        {component?.componentPathInfo.componentPathNames.join(' > ')}
      </div>
      <div style={rowCss}>
        <span style={subtleCss}>
          <AccessTimeIcon sx={rowIconCss} />
        </span>
        <span style={subtleCss}>{bug.issueState.status}</span>
      </div>
      <div style={rowCss}>
        <span style={subtleCss}>
          <InfoOutlinedIcon sx={rowIconCss} />
        </span>
        <span style={subtleCss}>Assignee</span>
        <span>{bug.issueState.assignee.emailAddress}</span>
      </div>
      <div style={rowCss}>
        <span style={{ width: '18px' }}></span>
        <span style={subtleCss}>Type</span>
        <span>{bug.issueState.type}</span>
        <span>{bug.issueState.priority}</span>
        <span>{bug.issueState.severity}</span>
      </div>
      <div style={rowCss}>
        <span style={subtleCss}>
          <LinkIcon sx={rowIconCss} />
        </span>
        <span>
          <a href={`https://issuetracker.google.com/${bugId}`}>Buganizer</a>
          {/* More links can be added here */}
        </span>
      </div>
    </div>
  );
};

// Styles used in the bug card.
const titleCss: CSSProperties = { fontWeight: 700 };
const subtleCss: CSSProperties = { opacity: 0.8 };
const rowCss: CSSProperties = {
  display: 'flex',
  gap: '12px',
  alignItems: 'center',
};
const rowIconCss: CSSProperties = { fontSize: '18px' };
