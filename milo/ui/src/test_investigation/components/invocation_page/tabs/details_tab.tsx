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

import { Box, Link, Typography } from '@mui/material';
import { DateTime } from 'luxon';

import { Timestamp } from '@/common/components/timestamp';
import { CodeMirrorEditor } from '@/generic_libs/components/code_mirror_editor';
import { useDeclareTabId } from '@/generic_libs/components/routed_tabs';
import { useInvocation } from '@/test_investigation/context/context';
import { isRootInvocation } from '@/test_investigation/utils/invocation_utils';

function isValidUrl(s: string) {
  try {
    new URL(s);
    return true;
  } catch {
    return false;
  }
}

interface KeyValueRowProps {
  label: string;
  value: React.ReactNode;
}

function KeyValueRow({ label, value }: KeyValueRowProps) {
  return (
    <>
      <Typography
        variant="body2"
        sx={{
          fontWeight: 'bold',
          color: 'text.primary',
          whiteSpace: 'nowrap',
        }}
      >
        {label}
      </Typography>
      <Typography
        variant="body2"
        component="div"
        sx={{ color: 'text.secondary', wordBreak: 'break-all' }}
      >
        {value}
      </Typography>
    </>
  );
}

export function DetailsTab() {
  useDeclareTabId('details');
  const invocation = useInvocation();

  const properties = invocation.properties || {};
  const formattedProperties = JSON.stringify(properties, null, 2);
  const tags = invocation.tags || [];

  const isRoot = isRootInvocation(invocation);

  return (
    <Box sx={{ p: 2, display: 'flex', flexDirection: 'column', gap: 4 }}>
      {/* Timing Section */}
      <Box>
        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: 'max-content 1fr',
            columnGap: 4,
            rowGap: 1,
            alignItems: 'baseline',
          }}
        >
          {invocation.createTime && (
            <KeyValueRow
              label="Created"
              value={
                <Timestamp
                  datetime={DateTime.fromISO(invocation.createTime as string)}
                />
              }
            />
          )}
          {isRoot && invocation.lastUpdated && (
            <KeyValueRow
              label="Last Updated"
              value={
                <Timestamp
                  datetime={DateTime.fromISO(invocation.lastUpdated as string)}
                />
              }
            />
          )}
          {invocation.finalizeStartTime && (
            <KeyValueRow
              label="Finalization Started"
              value={
                <Timestamp
                  datetime={DateTime.fromISO(
                    invocation.finalizeStartTime as string,
                  )}
                />
              }
            />
          )}
          {invocation.finalizeTime && (
            <KeyValueRow
              label="Finalized"
              value={
                <Timestamp
                  datetime={DateTime.fromISO(invocation.finalizeTime as string)}
                />
              }
            />
          )}
        </Box>
      </Box>

      {/* Properties Section */}
      <Box>
        <Typography variant="h6" gutterBottom>
          Properties
        </Typography>
        <CodeMirrorEditor
          value={formattedProperties}
          initOptions={{
            mode: 'javascript',
            readOnly: true,
            lineNumbers: true,
            foldGutter: true,
            gutters: ['CodeMirror-linenumbers', 'CodeMirror-foldgutter'],
          }}
        />
      </Box>

      {/* Tags Section */}
      <Box>
        <Typography variant="h6" gutterBottom>
          Tags
        </Typography>
        {tags.length > 0 ? (
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: 'max-content 1fr',
              columnGap: 4,
              rowGap: 1,
              alignItems: 'baseline',
            }}
          >
            {tags.map((tag, index) => (
              <KeyValueRow
                key={`${tag.key}-${tag.value}-${index}`}
                label={tag.key}
                value={
                  isValidUrl(tag.value) ? (
                    <Link
                      href={tag.value}
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {tag.value}
                    </Link>
                  ) : (
                    tag.value
                  )
                }
              />
            ))}
          </Box>
        ) : (
          <Typography color="text.secondary">No tags found.</Typography>
        )}
      </Box>
    </Box>
  );
}
