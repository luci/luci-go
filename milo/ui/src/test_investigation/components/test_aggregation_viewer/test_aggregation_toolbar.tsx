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
import LoadingButton from '@mui/lab/LoadingButton';
import type { SelectChangeEvent } from '@mui/material';
import {
  Box,
  IconButton,
  Typography,
  Link,
  Select,
  MenuItem,
  Checkbox,
  ListItemText,
  FormControl,
  InputLabel,
} from '@mui/material';

import { Aip160Autocomplete } from '@/common/components/aip_160_autocomplete';
import {
  FieldDef,
  ValueDef,
} from '@/common/components/aip_160_autocomplete/types';
import { HtmlTooltip } from '@/common/components/html_tooltip';
import { CategoryOption } from '@/generic_libs/components/filter/multi_select_category_chip';
import { TestResult_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_result.pb';
import { TestVerdict_Status } from '@/proto/go.chromium.org/luci/resultdb/proto/v1/test_verdict.pb';

import { useTestAggregationContext } from './context';

const FILTER_SCHEMA: FieldDef = {
  staticFields: {
    test_id: {},
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
    tags: {},
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
  },
};

const STATUS_OPTIONS: CategoryOption[] = [
  { value: TestVerdict_Status[TestVerdict_Status.FAILED], label: 'Failed' },
  {
    value: TestVerdict_Status[TestVerdict_Status.EXECUTION_ERRORED],
    label: 'Execution Errored',
  },
  { value: TestVerdict_Status[TestVerdict_Status.FLAKY], label: 'Flaky' },
  { value: TestVerdict_Status[TestVerdict_Status.PASSED], label: 'Passed' },
  { value: TestVerdict_Status[TestVerdict_Status.SKIPPED], label: 'Skipped' },
  {
    value: TestVerdict_Status[TestVerdict_Status.PRECLUDED],
    label: 'Precluded',
  },
];

export interface TestAggregationToolbarProps {
  onLocateCurrentTest?: () => void;
}

export function TestAggregationToolbar({
  onLocateCurrentTest,
}: TestAggregationToolbarProps) {
  const {
    selectedStatuses,
    setSelectedStatuses,
    aipFilter,
    setAipFilter,
    triggerLoadMore,
    loadedCount,
    isLoadingMore,
  } = useTestAggregationContext();

  const handleLoadMore = () => {
    triggerLoadMore();
  };

  const handleStatusChange = (event: SelectChangeEvent<string[]>) => {
    const {
      target: { value },
    } = event;
    const items = typeof value === 'string' ? value.split(',') : value;
    setSelectedStatuses(new Set(items));
  };

  return (
    <Box
      sx={{
        p: 1,
        borderBottom: '1px solid',
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
    >
      {/* Row 1: Status Dropdown */}
      <FormControl size="small" sx={{ width: '100%' }}>
        <InputLabel id="status-filter-label" sx={{ fontSize: '0.875rem' }}>
          Status
        </InputLabel>
        <Select
          labelId="status-filter-label"
          id="status-select"
          multiple
          value={Array.from(selectedStatuses)}
          onChange={handleStatusChange}
          label="Status"
          sx={{ fontSize: '0.875rem' }}
          renderValue={(selected) => {
            let labelText = '';
            if (selected.length === 0) labelText = 'All';
            else if (selected.length === STATUS_OPTIONS.length)
              labelText = 'All';
            else {
              labelText = selected
                .map(
                  (val) =>
                    STATUS_OPTIONS.find((opt) => opt.value === val)?.label ||
                    val,
                )
                .join(', ');
            }
            return <Typography variant="body2">{labelText}</Typography>;
          }}
        >
          {STATUS_OPTIONS.map((option) => (
            <MenuItem key={option.value} value={option.value} dense>
              <Checkbox
                checked={selectedStatuses.has(option.value)}
                size="small"
              />
              <ListItemText
                primary={option.label}
                primaryTypographyProps={{ variant: 'body2' }}
              />
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      {/* Row 2: AIP Filter & Help */}
      <Box
        sx={{ display: 'flex', gap: 0.5, width: '100%', alignItems: 'center' }}
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
                <code>status:FAILED AND duration&gt;50</code>
                <br />
                <code>
                  test_id:&quot;my_test&quot; OR
                  test_metadata.name:&quot;my_test&quot;
                </code>
              </Typography>
              <Typography variant="body2" component="div" sx={{ mb: 1 }}>
                Limits results to only those test verdicts that <em>contain</em>{' '}
                a test result matching this filter.
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
                    the structured form test ID
                  </li>
                  <li>
                    <strong>test_id_structured.module_scheme</strong> (string)
                  </li>
                  <li>
                    <strong>test_id_structured.module_variant</strong> (filters
                    behave as if this field is a map&lt;string, string&gt;)
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
                    <strong>test_metadata.name</strong> (string)
                  </li>
                  <li>
                    <strong>tags</strong> (repeated (key string, value string))
                  </li>
                  <li>
                    <strong>test_metadata.location.repo</strong> (string)
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

      {/* Row 3: Actions */}
      <Box
        sx={{
          display: 'flex',
          gap: 0.5,
          alignItems: 'center',
          flexWrap: 'wrap',
        }}
      >
        <HtmlTooltip title="Locate current test">
          <span>
            <IconButton
              size="small"
              onClick={onLocateCurrentTest}
              disabled={!onLocateCurrentTest}
              aria-label="Locate current test"
            >
              <GpsFixed fontSize="small" />
            </IconButton>
          </span>
        </HtmlTooltip>
        <LoadingButton
          variant="outlined"
          size="small"
          onClick={handleLoadMore}
          loading={isLoadingMore}
          sx={{ fontSize: '0.8125rem' }}
        >
          Load more test results ({loadedCount})
        </LoadingButton>
      </Box>
    </Box>
  );
}
