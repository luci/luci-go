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

import {
  LogsEntryTableCell,
  LogsHeaderCell,
  LogsTableEntry,
  SortOrder,
  VirtualizedTable,
} from '@chopsui/log-viewer';
import { LinearProgress, TableRow, Tooltip } from '@mui/material';
import { useQuery } from '@tanstack/react-query';
import { DateTime } from 'luxon';
import { useEffect, useMemo, useRef } from 'react';
import { TableVirtuosoHandle } from 'react-virtuoso';

import { Timestamp } from '@/common/components/timestamp';
import { NUMERIC_TIME_FORMAT_WITH_MS } from '@/common/tools/time_utils';
import { useSyncedSearchParams } from '@/generic_libs/hooks/synced_search_params';
import {
  ArtifactLine,
  artifactLine_SeverityToJSON,
} from '@/proto/go.chromium.org/luci/resultdb/proto/v1/artifact.pb';
import { ListArtifactLinesRequest } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/resultdb.pb';
import { useResultDbClient } from '@/test_verdict/hooks/prpc_clients';

import { useSelectedArtifact } from './context';

const EMPTY_CELL_VALUE = '-';
const LOG_SEARCH_QUERY = 'logSearchQuery';

interface LogSeverityProps {
  severity: string | undefined;
}

function LogSeverity({ severity }: LogSeverityProps) {
  if (!severity || severity === 'UNSPECIFIED') {
    return EMPTY_CELL_VALUE;
  }

  return (
    <Tooltip title={severity}>
      <div className={`severity ${severity.toLocaleLowerCase()}`}>
        {severity.charAt(0)}
      </div>
    </Tooltip>
  );
}

interface LogTableRowProps {
  entry: LogsTableEntry;
}

function LogTableRow({ entry }: LogTableRowProps) {
  return (
    <>
      <LogsEntryTableCell>{entry.entryId}</LogsEntryTableCell>
      <LogsEntryTableCell>
        <LogSeverity severity={entry.severity} />
      </LogsEntryTableCell>
      <LogsEntryTableCell>
        {entry.timestamp ? (
          <Timestamp
            datetime={DateTime.fromISO(entry.timestamp)}
            format={NUMERIC_TIME_FORMAT_WITH_MS}
          />
        ) : (
          '-'
        )}
      </LogsEntryTableCell>
      <LogsEntryTableCell>
        <div className="text-cell summary">{entry.summary}</div>
      </LogsEntryTableCell>
    </>
  );
}

function processLines(lines: readonly ArtifactLine[]) {
  const decoder = new TextDecoder();

  return lines.map((line) => {
    const ret: LogsTableEntry = {
      entryId: line.number,
      summary: decoder.decode(line.content),
      severity: artifactLine_SeverityToJSON(line.severity),
      timestamp: line.timestamp,
    };
    return ret;
  });
}

export function LogTable() {
  const selectedArtifact = useSelectedArtifact();
  const client = useResultDbClient();
  const virtuosoRef = useRef<TableVirtuosoHandle | null>(null);
  const [urlSearchParams] = useSyncedSearchParams();

  const { data, isLoading, isError, error } = useQuery({
    ...client.ListArtifactLines.query(
      ListArtifactLinesRequest.fromPartial({
        parent: selectedArtifact?.name || '',
      }),
    ),
    enabled: !!selectedArtifact,
  });
  const entries = useMemo(() => (data ? processLines(data.lines) : []), [data]);
  const searchTerm = urlSearchParams.get(LOG_SEARCH_QUERY);

  useEffect(() => {
    // Updating the search term in the URL will cause the table
    // to scroll to the first item matching that search term.
    if (entries.length > 0 && searchTerm && virtuosoRef.current) {
      virtuosoRef.current.scrollToIndex(
        entries.findIndex((l) => l.summary.includes(searchTerm)),
      );
    }
  }, [entries, searchTerm]);

  if (isError) {
    throw error;
  }

  return (
    <>
      {selectedArtifact && isLoading && <LinearProgress />}
      {!selectedArtifact ? (
        "Please select an artifact from the tree to view it's log lines."
      ) : (
        <>
          <VirtualizedTable
            ref={virtuosoRef}
            initialTopMostItemIndex={0}
            disableVirtualization={data && data.lines.length < 1500}
            entries={entries}
            fixedHeaderContent={() => (
              <TableRow>
                <LogsHeaderCell label="#" width="10px" />
                <LogsHeaderCell label="S" width="2rem" />
                <LogsHeaderCell
                  label="TIMESTAMP"
                  width="11rem"
                  // TODO (b/356960468): Implement sorting.
                  sortId="timestamp"
                  sortOrder={SortOrder.ASC}
                  onHeaderSort={() => {}}
                />
                <LogsHeaderCell label="" />
              </TableRow>
            )}
            rowContent={(_, row) => {
              return <LogTableRow entry={row} />;
            }}
          />
        </>
      )}
    </>
  );
}
