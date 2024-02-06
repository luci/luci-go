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

import { Box, Button, FormControl, TextField } from '@mui/material';
import { isEqual } from 'lodash-es';
import { useRef, useState } from 'react';

import { ChangepointPredicate } from '@/proto/go.chromium.org/luci/analysis/proto/v1/changepoints.pb';

export interface RegressionFiltersProps {
  readonly predicate: ChangepointPredicate;
  readonly onPredicateUpdate: (predicate: ChangepointPredicate) => void;
}

export function RegressionFilters({
  predicate,
  onPredicateUpdate,
}: RegressionFiltersProps) {
  const [pendingPredicate, setPendingPredicate] = useState(predicate);
  const noUncommittedUpdate = isEqual(predicate, pendingPredicate);

  const predicateRef = useRef(predicate);

  // The predicate provided by the parent should be treated as the source of
  // truth. If the parent provides a new predicate, discard the pending
  // predicate by syncing it with the current predicate.
  if (!isEqual(predicate, predicateRef.current)) {
    predicateRef.current = predicate;

    // Set the pending predicate in the rendering cycle so we don't render the
    // stale predicate when the parent triggers an update.
    setPendingPredicate(predicate);
  }

  function handlePendingPredicateUpdate(
    newPendingPredicate: ChangepointPredicate,
  ) {
    if (isEqual(newPendingPredicate, pendingPredicate)) {
      return;
    }
    setPendingPredicate(newPendingPredicate);
  }

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: '1fr auto',
        gap: '5px',
        width: '80%',
        maxWidth: '1200px',
      }}
    >
      <FormControl fullWidth>
        <TextField
          label="Test ID prefix"
          fullWidth
          size="small"
          value={pendingPredicate.testIdPrefix}
          onChange={(e) =>
            handlePendingPredicateUpdate({
              ...predicate,
              testIdPrefix: e.target.value,
            })
          }
        />
      </FormControl>
      <Button
        size="small"
        variant="contained"
        disabled={noUncommittedUpdate}
        title={
          noUncommittedUpdate
            ? 'The filter has already been applied.'
            : 'Click to apply the filter.'
        }
        onClick={() => onPredicateUpdate(pendingPredicate)}
      >
        Apply Filter
      </Button>
    </Box>
  );
}
