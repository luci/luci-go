import {
  ResourceRequest,
  ResourceRequest_Status,
} from '@/proto/go.chromium.org/infra/fleetconsole/api/fleetconsolerpc/service.pb';

import { RRI_COLUMNS, RriGridRow } from './rri_columns';

describe('RRI_COLUMNS', () => {
  const slippageColumn = RRI_COLUMNS.find((c) => c.id === 'slippage')!;

  it('calculates slippage for incomplete items', () => {
    const row = {} as RriGridRow;
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.IN_PROGRESS,
      resourceRequestTargetDeliveryDate: { year: 2025, month: 3, day: 20 },
      resourceRequestActualDeliveryDate: { year: 2025, month: 3, day: 25 },
    } as ResourceRequest;

    slippageColumn.assignValue(rr, row);
    expect(row.slippage).toBe(5);
  });

  it('calculates slippage for completed items (LATE)', () => {
    const row = {} as RriGridRow;
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.COMPLETE,
      resourceRequestTargetDeliveryDate: { year: 2025, month: 3, day: 20 },
      resourceRequestActualDeliveryDate: { year: 2025, month: 3, day: 25 },
    } as ResourceRequest;

    slippageColumn.assignValue(rr, row);
    expect(row.slippage).toBe(5);
  });

  it('calculates slippage for completed items (EARLY)', () => {
    const row = {} as RriGridRow;
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.COMPLETE,
      resourceRequestTargetDeliveryDate: { year: 2025, month: 3, day: 25 },
      resourceRequestActualDeliveryDate: { year: 2025, month: 3, day: 20 },
    } as ResourceRequest;

    slippageColumn.assignValue(rr, row);
    expect(row.slippage).toBe(-5);
  });

  it('calculates 0 slippage if dates are missing', () => {
    const row = {} as RriGridRow;
    const rr = {
      fulfillmentStatus: ResourceRequest_Status.COMPLETE,
    } as ResourceRequest;

    slippageColumn.assignValue(rr, row);
    expect(row.slippage).toBe(0);
  });
});
