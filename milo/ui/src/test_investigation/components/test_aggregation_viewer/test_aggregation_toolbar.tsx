// Copyright 2026 The LUCI Authors.
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

import { GpsFixed } from '@mui/icons-material';
import HelpOutlineIcon from '@mui/icons-material/HelpOutline';
import { Box, IconButton, Typography, Link } from '@mui/material';

import { Aip160Autocomplete } from '@/common/components/aip_160_autocomplete';
import {
  FieldDef,
  ValueDef,
} from '@/common/components/aip_160_autocomplete/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';

import { useTestAggregationContext } from './context';
import { StatusFilterDropdown } from './status_filter_dropdown';

const FILTER_SCHEMA: FieldDef = {
  staticFields: {
    test_id: {},
    test_id_structured: {
      staticFields: {
        module_name: {},
        module_scheme: {},
        module_variant: {},
        module_variant_hash: {},
        coarse_name: {},
        fine_name: {},
        case_name: {},
      },
    },
    test_metadata: {
      staticFields: {
        name: {},
        location: {
          staticFields: {
            repo: {},
            file_name: {},
          },
        },
      },
    },
    tags: {},
    status: {
      getValues: (partial: string): readonly ValueDef[] => {
        const partialUpper = partial.toUpperCase();
        return Object.keys(TestResult_Status)
          .filter((k) => isNaN(Number(k)) && k !== 'STATUS_UNSPECIFIED')
          .filter((k) => k.includes(partialUpper))
          .map((k) => ({ text: k }));
      },
    },
    duration: {},
  },
};

export interface TestAggregationToolbarProps {
  onLocateCurrentTest?: () => void;
}

export function TestAggregationToolbar({
  onLocateCurrentTest,
}: TestAggregationToolbarProps) {
  const { aipFilter, setAipFilter } = useTestAggregationContext();

  return (
    <Box
      sx={{
        p: 1,
        borderBottom: '1px solid',
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'row',
        flexWrap: 'wrap',
        gap: 1,
        alignItems: 'center',
      }}
    >
      {/* Filters Section - Grows to fill available space */}
      <Box
        sx={{
          display: 'flex',
          gap: 1,
          flexGrow: 1,
          flexWrap: 'wrap',
          alignItems: 'center',
          minWidth: '300px', // Prevent collapsing too small
        }}
      >
        {onLocateCurrentTest && (
          <HtmlTooltip title="Locate current test">
            <span>
              <IconButton
                size="small"
                onClick={onLocateCurrentTest}
                aria-label="Locate current test"
              >
                <GpsFixed fontSize="small" />
              </IconButton>
            </span>
          </HtmlTooltip>
        )}
        <StatusFilterDropdown />
        <Box
          sx={{
            display: 'flex',
            gap: 0.5,
            flexGrow: 1,
            alignItems: 'center',
            minWidth: '250px',
          }}
        >
          <Aip160Autocomplete
            schema={FILTER_SCHEMA}
            value={aipFilter}
            onValueCommit={setAipFilter}
            placeholder="e.g. status:FAILED AND duration>50"
            sx={{
              flexGrow: 1,
              '& .MuiInputBase-input': { fontSize: '0.875rem' },
              '& .MuiInputLabel-root': { fontSize: '0.875rem' },
            }}
            slotProps={{
              textField: {
                size: 'small',
                label: 'Contains test results matching...',
              },
            }}
          />
          <HtmlTooltip
            title={
              <>
                <Typography variant="body2" sx={{ mb: 1 }}>
                  Example filters:
                  <br />
                  <code>
                    test_id_structured.module_name=&quot;foo&quot; AND
                    status=FAILED
                  </code>
                  <br />
                  <code>
                    test_id:&quot;my_test&quot; OR
                    test_metadata.name:&quot;my_test&quot;
                  </code>
                </Typography>
                <Typography variant="body2" component="div" sx={{ mb: 1 }}>
                  Module, coarse and fine aggregations will only be returned if
                  they contain at least one verdict or module matching this
                  filter, where:
                  <ul style={{ margin: 0, paddingLeft: '20px' }}>
                    <li>
                      A test verdict is only matched if it contains a test
                      result which matches this filter.
                    </li>
                    <li>
                      A module is only matched if it matches this filter, or
                      contains a test result which matches this filter.
                    </li>
                  </ul>
                </Typography>
                <Typography variant="body2" component="div" sx={{ mb: 1 }}>
                  For modules, only the fields available are{' '}
                  <code>test_id_structured.module_name</code>,{' '}
                  <code>test_id_structured.module_scheme</code>,{' '}
                  <code>test_id_structured.module_variant</code>, and{' '}
                  <code>test_id_structured.module_variant_hash</code>. If other
                  fields are used, the search query will never match a module
                  directly, however, a module will still be matched if one of
                  its test results matches the filter.
                </Typography>
                <Typography variant="body2" component="div" sx={{ mb: 1 }}>
                  The filter is an AIP-160 filter string (see{' '}
                  <Link
                    href="https://google.aip.dev/160"
                    target="_blank"
                    rel="noopener"
                    underline="always"
                  >
                    https://google.aip.dev/160
                  </Link>{' '}
                  for syntax), with the following fields available:
                  <ul style={{ margin: 0, paddingLeft: '20px' }}>
                    <li>
                      <strong>test_id</strong> (string) - the flat-form test ID
                    </li>
                    <li>
                      <strong>test_id_structured.module_name</strong> (string) -
                      the structured form test ID fields
                    </li>
                    <li>
                      <strong>test_id_structured.module_scheme</strong> (string)
                    </li>
                    <li>
                      <strong>test_id_structured.module_variant</strong>{' '}
                      (filters behave as if this field is a map&lt;string,
                      string&gt;)
                    </li>
                    <li>
                      <strong>test_id_structured.module_variant_hash</strong>{' '}
                      (string)
                    </li>
                    <li>
                      <strong>test_id_structured.coarse_name</strong> (string)
                    </li>
                    <li>
                      <strong>test_id_structured.fine_name</strong> (string)
                    </li>
                    <li>
                      <strong>test_id_structured.case_name</strong> (string)
                    </li>
                    <li>
                      <strong>test_metadata.name</strong> (string) - the
                      original test name
                    </li>
                    <li>
                      <strong>tags</strong> (repeated (key string, value
                      string)) - the test tags
                    </li>
                    <li>
                      <strong>test_metadata.location.repo</strong> (string) -
                      the test metadata location
                    </li>
                    <li>
                      <strong>test_metadata.location.file_name</strong> (string)
                    </li>
                    <li>
                      <strong>status</strong> (enum
                      luci.resultdb.v1.TestResult.Status) - the status_v2 of the
                      test result
                    </li>
                    <li>
                      <strong>duration</strong> (google.protobuf.Duration)
                    </li>
                  </ul>
                </Typography>
              </>
            }
          >
            <IconButton size="small">
              <HelpOutlineIcon fontSize="small" />
            </IconButton>
          </HtmlTooltip>
        </Box>
      </Box>
    </Box>
  );
}
