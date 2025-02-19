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

import HelpOutline from '@mui/icons-material/HelpOutline';
import Search from '@mui/icons-material/Search';
import IconButton from '@mui/material/IconButton';
import InputAdornment from '@mui/material/InputAdornment';
import Popover from '@mui/material/Popover';
import Typography from '@mui/material/Typography';
import { useMemo, useRef, useState } from 'react';

import { useTestHistoryClient } from '@/analysis/hooks/prpc_clients';
import { useFilterParam } from '@/clusters/components/clusters_table/hooks';
import {
  Aip160Autocomplete,
  FetchValuesFn,
  FieldsSchema,
} from '@/common/components/aip_160_filter';
import { CommitOrClear } from '@/generic_libs/components/text_autocomplete';
import {
  QueryTestsRequest,
  QueryTestsResponse,
} from '@/proto/go.chromium.org/luci/analysis/proto/v1/test_history.pb';

const FilterHelp = () => {
  // TODO: more styling on this.
  return (
    <Typography sx={{ p: 2, maxWidth: '800px' }}>
      <p>
        Searching will display clusters and cluster impact based only on test
        failures that match your search.
      </p>
      <p>
        Searching supports a subset of{' '}
        <a href="https://google.aip.dev/160">AIP-160 filtering</a>.
      </p>
      <p>
        A bare value is searched for in the columns test_id and failure_reason.
        Values are case-sensitive. E.g. <b>ninja</b> or{' '}
        <b>&ldquo;test failed&rdquo;</b>.
      </p>
      <p>
        You can use AND, OR and NOT (case sensitive) logical operators, along
        with grouping. &lsquo;-&rsquo; is equivalent to NOT. Multiple bare
        values are considered to be AND separated. These are equivalent:{' '}
        <b>hello world</b> and <b>hello AND world</b>. More examples:{' '}
        <b>a OR b</b> or <b>a AND NOT(b or -c)</b>.
      </p>
      <p>
        You can search particular columns with &lsquo;=&rsquo;, &lsquo;!=&rsquo;
        and &lsquo;:&rsquo; (has) operators. The right hand side of the operator
        must be a simple value. E.g. <b>test_id:telemetry</b>,{' '}
        <b>-failure_reason:Timeout</b>, <b>tags.monorail_component=Blink</b> or{' '}
        <b>ingested_invocation_id=&ldquo;build-8822963500388678513&rdquo;</b>.
      </p>
      <p>
        Supported columns to search on:
        <ul>
          <li>test_id</li>
          <li>failure_reason</li>
          <li>realm</li>
          <li>ingested_invocation_id</li>
          <li>cluster_algorithm</li>
          <li>cluster_id</li>
          <li>variant_hash</li>
          <li>test_run_id</li>
          <li>is_test_run_blocked</li>
          <li>is_ingested_invocation_blocked</li>
        </ul>
      </p>
      <p>
        You can also search based on particular variant or tag key/value pairs.
        E.g. <b>tags.team_email:device-dev</b> or <b>variant.os:Ubuntu</b>
      </p>
    </Typography>
  );
};

export interface ClustersTableFilterProps {
  readonly project: string;
}

const ClustersTableFilter = ({ project }: ClustersTableFilterProps) => {
  const [filterHelpAnchorEl, setFilterHelpAnchorEl] =
    useState<HTMLButtonElement | null>(null);
  const [failureFilter, updateFailureFilterParam] = useFilterParam();

  const client = useTestHistoryClient();
  const fetchTestIds: FetchValuesFn<QueryTestsResponse> = (
    partial: string,
  ) => ({
    ...client.QueryTests.query(
      QueryTestsRequest.fromPartial({
        project,
        testIdSubstring: partial,
        caseInsensitive: true,
        pageSize: 50,
      }),
    ),
    enabled: partial !== '',
    select: (data) => {
      return data.testIds
        .filter((text) => text !== partial)
        .map((text) => ({ text }));
    },
  });
  const fetchTestIdsRef = useRef(fetchTestIds);
  fetchTestIdsRef.current = fetchTestIds;

  const schema = useMemo(() => {
    const ret: FieldsSchema = {
      test_id: {
        fetchValues: (...params) => fetchTestIdsRef.current(...params),
      },
      failure_reason: {},
      realm: {},
      ingested_invocation_id: {},
      cluster_algorithm: {
        getValues: (partial) => {
          const lowerPartial = partial.toLowerCase();
          return ['rules', 'rules-v3', 'reason-v6', 'testname-v4']
            .filter((text) => text.includes(lowerPartial))
            .map((text) => ({ text }));
        },
      },
      cluster_id: {},
      variant_hash: {},
      test_run_id: {},
      is_test_run_blocked: {
        getValues: (partial) => {
          const lowerPartial = partial.toLowerCase();
          return ['true', 'false']
            .filter((text) => text.includes(lowerPartial))
            .map((text) => ({ text }));
        },
      },
      is_ingested_invocation_blocked: {
        getValues: (partial) => {
          const lowerPartial = partial.toLowerCase();
          return ['true', 'false']
            .filter((text) => text.includes(lowerPartial))
            .map((text) => ({ text }));
        },
      },
    };
    return ret;
  }, []);

  return (
    <>
      <Aip160Autocomplete
        schema={schema}
        value={failureFilter}
        onValueCommit={(newVal) => updateFailureFilterParam(newVal)}
        placeholder="Filter test failures used in clusters"
        slotProps={{
          textField: {
            slotProps: {
              input: {
                startAdornment: (
                  <InputAdornment position="start">
                    <Search />
                  </InputAdornment>
                ),
                endAdornment: (
                  <InputAdornment position="end">
                    <CommitOrClear />
                    <IconButton
                      aria-label="toggle search help"
                      edge="end"
                      onClick={(e) => setFilterHelpAnchorEl(e.currentTarget)}
                    >
                      <HelpOutline />
                    </IconButton>
                  </InputAdornment>
                ),
                inputProps: {
                  'data-testid': 'failure_filter_input',
                },
              },
            },
          },
        }}
      />
      <Popover
        open={Boolean(filterHelpAnchorEl)}
        anchorEl={filterHelpAnchorEl}
        onClose={() => setFilterHelpAnchorEl(null)}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
      >
        <FilterHelp />
      </Popover>
    </>
  );
};

export default ClustersTableFilter;
