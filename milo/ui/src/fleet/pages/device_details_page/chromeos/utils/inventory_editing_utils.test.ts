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

import { MachineLSE } from '@/proto/go.chromium.org/infra/unifiedfleet/api/v1/models/machine_lse.pb';

import {
  areArraysEqual,
  calculateDiff,
  getNestedValue,
  mutateNestedValue,
  translateDiffToEdits,
  updateNestedValues,
  generateShivasCommands,
  generateChangelogMarkdown,
  hasDeployableEdits,
  getEditableFields,
  checkLengthLimit,
} from './inventory_editing_utils';

describe('inventory_editing_utils', () => {
  describe('getNestedValue', () => {
    it('gets a nested property by string path', () => {
      const obj = {
        a: {
          b: {
            c: 'val',
          },
        },
      };
      expect(getNestedValue(obj, 'a.b.c')).toBe('val');
    });

    it('returns undefined if path is missing', () => {
      const obj = { a: {} };
      expect(getNestedValue(obj, 'a.b.c')).toBeUndefined();
    });
  });

  describe('checkLengthLimit', () => {
    it('returns true for undefined or null or empty', () => {
      expect(checkLengthLimit(undefined)).toBe(true);
      expect(checkLengthLimit(null)).toBe(true);
      expect(checkLengthLimit('')).toBe(true);
    });

    it('validates string length', () => {
      expect(checkLengthLimit('short', 10)).toBe(true);
      expect(checkLengthLimit('too-long-string', 10)).toBe(false);
    });
  });

  describe('mutateNestedValue', () => {
    it('mutates a nested property by path', () => {
      const obj = {
        a: {
          b: {
            c: 'old',
          },
        },
      };
      mutateNestedValue(obj, 'a.b.c', 'new');
      expect(obj.a.b.c).toBe('new');
    });

    it('creates intermediate objects if path segments are missing', () => {
      const obj: Record<string, unknown> = {};
      mutateNestedValue(obj, 'a.b.c', 'new');
      const a = obj.a as Record<string, unknown>;
      const b = a.b as Record<string, unknown>;
      expect(b.c).toBe('new');
    });
  });

  describe('updateNestedValues', () => {
    it('returns a cloned object with multiple nested changes', () => {
      const obj = {
        a: { b: '1' },
        x: { y: '2' },
      };
      interface ExpectedResult {
        a: { b: string };
        x: { y: string };
      }
      const result = updateNestedValues(obj, [
        { path: 'a.b', value: '11' },
        { path: 'x.y', value: '22' },
      ]) as unknown as ExpectedResult;

      expect(result).not.toBe(obj);
      expect(result.a.b).toBe('11');
      expect(result.x.y).toBe('22');
      expect(obj.a.b).toBe('1'); // original is unchanged
    });
  });

  describe('areArraysEqual', () => {
    it('returns true if arrays are equal', () => {
      expect(areArraysEqual(['a', 'b'], ['a', 'b'])).toBe(true);
    });

    it('returns false if lengths differ', () => {
      expect(areArraysEqual(['a'], ['a', 'b'])).toBe(false);
    });

    it('returns true if values differ in order but are equivalent sets', () => {
      expect(areArraysEqual(['a', 'b'], ['b', 'a'])).toBe(true);
    });
  });

  describe('calculateDiff', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('returns empty diff if no changes', () => {
      const updated = JSON.parse(JSON.stringify(original));
      expect(calculateDiff(original, updated)).toEqual([]);
    });

    it('returns diff when array fields change', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.dut!.pools = ['pool1', 'pool3'];

      expect(calculateDiff(original, updated)).toEqual([
        {
          path: 'Pools',
          original: 'pool1,pool2',
          updated: 'pool1,pool3',
        },
      ]);
    });
  });

  describe('translateDiffToEdits', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('translates changed array fields to DeviceConfigEdits format', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.dut!.pools = ['pool1', 'pool3'];

      const result = translateDiffToEdits(original, updated);
      expect(result.edits).toEqual({
        pools: ['pool1', 'pool3'],
      });
      expect(result.paths).toEqual(['pools']);
    });
  });

  describe('calculateDiff (Labstation)', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          labstation: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('returns empty diff if no changes', () => {
      const updated = JSON.parse(JSON.stringify(original));
      expect(calculateDiff(original, updated)).toEqual([]);
    });

    it('returns diff when array fields change', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.labstation!.pools = [
        'pool1',
        'pool3',
      ];

      expect(calculateDiff(original, updated)).toEqual([
        {
          path: 'Pools',
          original: 'pool1,pool2',
          updated: 'pool1,pool3',
        },
      ]);
    });
  });

  describe('translateDiffToEdits (Labstation)', () => {
    const original = {
      name: 'machineLSEs/test-device',
      chromeosMachineLse: {
        deviceLse: {
          labstation: {
            pools: ['pool1', 'pool2'],
          },
        },
      },
    } as unknown as MachineLSE;

    it('translates changed array fields to DeviceConfigEdits format', () => {
      const updated = JSON.parse(JSON.stringify(original));
      updated.chromeosMachineLse!.deviceLse!.labstation!.pools = [
        'pool1',
        'pool3',
      ];

      const result = translateDiffToEdits(original, updated);
      expect(result.edits).toEqual({
        pools: ['pool1', 'pool3'],
      });
      expect(result.paths).toEqual(['pools']);
    });
  });

  describe('generateShivasCommands', () => {
    const hostname = 'test-host';
    const ufsNamespace = 'os';

    describe('DUT', () => {
      const original = {
        name: 'machineLSEs/test-host',
        chromeosMachineLse: {
          deviceLse: {
            dut: {
              pools: ['pool1', 'pool2'],
            },
          },
        },
      } as unknown as MachineLSE;

      it('returns empty array if no changes', () => {
        const updated = JSON.parse(JSON.stringify(original));
        expect(
          generateShivasCommands(original, updated, hostname, ufsNamespace),
        ).toEqual([]);
      });

      it('generates update command when pools change', () => {
        const updated = JSON.parse(JSON.stringify(original));
        updated.chromeosMachineLse!.deviceLse!.dut!.pools = ['pool1', 'pool3'];

        expect(
          generateShivasCommands(original, updated, hostname, ufsNamespace),
        ).toEqual([
          'shivas update dut -name test-host -namespace os -pools-replace pool1,pool3',
        ]);
      });

      it('generates update command when pools are cleared', () => {
        const updated = JSON.parse(JSON.stringify(original));
        updated.chromeosMachineLse!.deviceLse!.dut!.pools = [];

        expect(
          generateShivasCommands(original, updated, hostname, ufsNamespace),
        ).toEqual([
          'shivas update dut -name test-host -namespace os -pools-replace -',
        ]);
      });
    });

    describe('Labstation', () => {
      const original = {
        name: 'machineLSEs/test-host',
        chromeosMachineLse: {
          deviceLse: {
            labstation: {
              pools: ['pool1', 'pool2'],
            },
          },
        },
      } as unknown as MachineLSE;

      it('generates update command for labstation pools', () => {
        const updated = JSON.parse(JSON.stringify(original));
        updated.chromeosMachineLse!.deviceLse!.labstation!.pools = [
          'pool1',
          'pool3',
        ];

        expect(
          generateShivasCommands(original, updated, hostname, ufsNamespace),
        ).toEqual([
          'shivas update labstation -name test-host -namespace os -pools-replace pool1,pool3',
        ]);
      });
    });
  });

  describe('generateChangelogMarkdown', () => {
    it('returns empty string if no diffs', () => {
      expect(generateChangelogMarkdown([], 'device-1')).toBe('');
    });

    it('generates markdown for diffs', () => {
      const diffs = [
        { path: 'Pools', original: 'pool1', updated: 'pool2' },
        { path: 'Hostname', original: 'old-host', updated: 'new-host' },
      ];
      const expected = [
        '**Inventory updates for device-1:**',
        '*   **Pools**: `pool1` ➔ `pool2`',
        '*   **Hostname**: `old-host` ➔ `new-host`',
      ].join('\n');

      expect(generateChangelogMarkdown(diffs, 'device-1')).toBe(expected);
    });
  });

  describe('Servo editing', () => {
    const original = {
      name: 'machineLSEs/test-host',
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            pools: ['pool1'],
            peripherals: {
              servo: {
                servoHostname: 'servo-old',
                servoPort: 9999,
                servoSerial: 'serial-old',
              },
            },
          },
        },
      },
    } as unknown as MachineLSE;

    const updated = JSON.parse(JSON.stringify(original));
    updated.chromeosMachineLse!.deviceLse!.dut!.peripherals!.servo = {
      servoHostname: 'servo-new',
      servoPort: 8888,
      servoSerial: 'serial-new',
    };

    it('calculates diffs for servo fields', () => {
      const diffs = calculateDiff(original, updated);
      expect(diffs).toContainEqual({
        path: 'Servo Hostname',
        original: 'servo-old',
        updated: 'servo-new',
      });
      expect(diffs).toContainEqual({
        path: 'Servo Port',
        original: '9999',
        updated: '8888',
      });
      expect(diffs).toContainEqual({
        path: 'Servo Serial',
        original: 'serial-old',
        updated: 'serial-new',
      });
    });

    it('translates diffs to DeviceConfigEdits format', () => {
      const { edits, paths } = translateDiffToEdits(original, updated);
      expect(edits.servo).toEqual({
        hostname: 'servo-new',
        port: 8888,
        serial: 'serial-new',
      });
      expect(paths).toEqual(['servo.hostname', 'servo.port', 'servo.serial']);
    });

    it('generates shivas update command for servo fields on a DUT', () => {
      const commands = generateShivasCommands(
        original,
        updated,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update dut -name test-host -namespace os -servo servo-new:8888 -servo-serial serial-new',
      ]);
    });
  });

  describe('RPM editing', () => {
    const original = {
      name: 'machineLSEs/test-host',
      chromeosMachineLse: {
        deviceLse: {
          dut: {
            peripherals: {
              rpm: {
                powerunitName: 'rpm-old',
                powerunitOutlet: 'outlet-old',
                powerunitType: 1, // TYPE_SENTRY
              },
            },
          },
        },
      },
    } as unknown as MachineLSE;

    const updated = JSON.parse(JSON.stringify(original));
    updated.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm = {
      powerunitName: 'rpm-new',
      powerunitOutlet: 'outlet-new',
      powerunitType: 2, // TYPE_IP9850
    };

    it('calculates diffs for rpm fields', () => {
      const diffs = calculateDiff(original, updated);
      expect(diffs).toContainEqual({
        path: 'RPM Hostname',
        original: 'rpm-old',
        updated: 'rpm-new',
      });
      expect(diffs).toContainEqual({
        path: 'RPM Outlet',
        original: 'outlet-old',
        updated: 'outlet-new',
      });
      expect(diffs).toContainEqual({
        path: 'RPM Type',
        original: 'SENTRY',
        updated: 'IP9850',
      });
    });

    it('translates diffs to DeviceConfigEdits format', () => {
      const { edits, paths } = translateDiffToEdits(original, updated);
      expect(edits.rpm).toEqual({
        host: 'rpm-new',
        outlet: 'outlet-new',
        type: 'ip9850',
      });
      expect(paths).toEqual(['rpm.host', 'rpm.outlet', 'rpm.type']);
    });

    it('explicitly clears rpm.outlet and removes rpm.type when rpm.host is cleared', () => {
      const clearedHost = JSON.parse(JSON.stringify(original));
      clearedHost.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm!.powerunitName =
        '';
      clearedHost.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm!.powerunitType = 0;

      const { edits, paths } = translateDiffToEdits(original, clearedHost);
      expect(edits.rpm).toEqual({
        host: '',
        outlet: '',
        type: '',
      });
      expect(paths).toEqual(['rpm.host', 'rpm.outlet']);
    });

    it('generates shivas update command for rpm fields on a DUT', () => {
      const commands = generateShivasCommands(
        original,
        updated,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update dut -name test-host -namespace os -rpm rpm-new -rpm-outlet outlet-new -rpm-type ip9850',
      ]);
    });

    it('generates shivas command with only changed RPM fields', () => {
      const partialUpdated = JSON.parse(JSON.stringify(original));
      partialUpdated.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm!.powerunitName =
        'rpm-new';

      const commands = generateShivasCommands(
        original,
        partialUpdated,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update dut -name test-host -namespace os -rpm rpm-new',
      ]);
    });

    it('generates shivas command to clear individual RPM fields', () => {
      const clearedFields = JSON.parse(JSON.stringify(original));
      clearedFields.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm!.powerunitOutlet =
        '';
      clearedFields.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm!.powerunitType = 0;

      const commands = generateShivasCommands(
        original,
        clearedFields,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update dut -name test-host -namespace os -rpm-outlet - -rpm-type unknown',
      ]);
    });

    it('generates shivas command to delete RPM entirely when host is cleared', () => {
      const deletedRpm = JSON.parse(JSON.stringify(original));
      deletedRpm.chromeosMachineLse!.deviceLse!.dut!.peripherals!.rpm!.powerunitName =
        '';

      const commands = generateShivasCommands(
        original,
        deletedRpm,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update dut -name test-host -namespace os -rpm -',
      ]);
    });
  });

  describe('hasDeployableEdits', () => {
    it('returns true if servo fields are edited on a DUT', () => {
      const diffs = [
        { path: 'Servo Hostname', original: 'old', updated: 'new' },
      ];
      expect(hasDeployableEdits(diffs, false)).toBe(true);
    });

    it('returns true if rpm fields are edited on a DUT', () => {
      const diffs = [{ path: 'RPM Hostname', original: 'old', updated: 'new' }];
      expect(hasDeployableEdits(diffs, false)).toBe(true);
    });

    it('returns false if only pools are edited on a DUT', () => {
      const diffs = [{ path: 'Pools', original: 'p1', updated: 'p2' }];
      expect(hasDeployableEdits(diffs, false)).toBe(false);
    });

    it('returns false if servo fields are edited on a Labstation', () => {
      const diffs = [
        { path: 'Servo Hostname', original: 'old', updated: 'new' },
      ];
      expect(hasDeployableEdits(diffs, true)).toBe(false);
    });
  });

  describe('translateDiffToEdits deterministic zero-clearing', () => {
    it('explicitly clears servo.port to 0 and includes it in paths when servo.hostname is cleared to empty string', () => {
      const original = {
        chromeosMachineLse: {
          deviceLse: {
            dut: {
              peripherals: {
                servo: {
                  servoHostname: 'old-servo',
                  servoPort: 9999,
                },
              },
            },
          },
        },
      } as unknown as MachineLSE;
      const updated = {
        chromeosMachineLse: {
          deviceLse: {
            dut: {
              peripherals: {
                servo: {
                  servoHostname: '',
                  servoPort: 9999,
                },
              },
            },
          },
        },
      } as unknown as MachineLSE;
      const { edits, paths } = translateDiffToEdits(original, updated);
      expect(edits.servo).toEqual({
        hostname: '',
        port: 0,
      });
      expect(paths).toContain('servo.hostname');
      expect(paths).toContain('servo.port');
    });
  });

  describe('getEditableFields', () => {
    it('includes numeric range validation properties for Servo Port on DUTs', () => {
      const fields = getEditableFields(false);
      const portField = fields.find((f) => f.editPath === 'servo.port');
      expect(portField).toBeDefined();
      expect(portField?.min).toBe(9000);
      expect(portField?.max).toBe(9999);
    });

    it('includes zone in editable fields', () => {
      const fields = getEditableFields(false);
      const zoneField = fields.find((f) => f.editPath === 'zone');
      expect(zoneField).toBeDefined();
      expect(zoneField?.path).toBe('zone');
    });

    it('includes rack in editable fields', () => {
      const fields = getEditableFields(false);
      const rackField = fields.find((f) => f.editPath === 'rack');
      expect(rackField).toBeDefined();
      expect(rackField?.path).toBe('rack');
    });
  });

  describe('Zone editing', () => {
    const original = {
      name: 'machineLSEs/test-host',
      zone: 'ZONE_CHROMEOS1',
      rack: 'chromeos1-row1-rack1',
      machines: ['chromeos-machine-1'],
    } as unknown as MachineLSE;

    const updated = {
      name: 'machineLSEs/test-host',
      zone: 'ZONE_ATLANTA',
      rack: 'chromeos1-row1-rack1',
      machines: ['chromeos-machine-1'],
    } as unknown as MachineLSE;

    it('translates diff to DeviceConfigEdits format and automatically includes rack when zone changes', () => {
      const { edits, paths } = translateDiffToEdits(original, updated);
      expect(edits.zone).toBe('ZONE_ATLANTA');
      expect(edits.rack).toBe('chromeos1-row1-rack1');
      expect(paths).toEqual(['zone', 'rack']);
    });

    it('generates shivas update machine command with -zone and -rack flags and machine name', () => {
      const commands = generateShivasCommands(
        original,
        updated,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update machine -name chromeos-machine-1 -namespace os -zone ZONE_ATLANTA -rack chromeos1-row1-rack1',
      ]);
    });

    it('generates shivas update machine command with -zone - when cleared and falls back to hostname when machines array is empty', () => {
      const origNoMachine = {
        name: 'machineLSEs/test-host',
        zone: 'ZONE_CHROMEOS1',
        rack: 'chromeos1-row1-rack1',
      } as unknown as MachineLSE;
      const cleared = {
        name: 'machineLSEs/test-host',
        zone: '',
        rack: 'chromeos1-row1-rack1',
      } as unknown as MachineLSE;
      const commands = generateShivasCommands(
        origNoMachine,
        cleared,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update machine -name test-host -namespace os -zone - -rack chromeos1-row1-rack1',
      ]);
    });

    it('generates both shivas update dut and shivas update machine commands when both dut fields and zone are updated', () => {
      const origMulti = {
        name: 'machineLSEs/test-host',
        zone: 'ZONE_CHROMEOS1',
        rack: 'chromeos1-row1-rack1',
        machines: ['chromeos-machine-1'],
        chromeosMachineLse: {
          deviceLse: {
            dut: {
              pools: ['pool1'],
            },
          },
        },
      } as unknown as MachineLSE;
      const updatedMulti = {
        name: 'machineLSEs/test-host',
        zone: 'ZONE_ATLANTA',
        rack: 'chromeos1-row1-rack1',
        machines: ['chromeos-machine-1'],
        chromeosMachineLse: {
          deviceLse: {
            dut: {
              pools: ['pool2'],
            },
          },
        },
      } as unknown as MachineLSE;
      const commands = generateShivasCommands(
        origMulti,
        updatedMulti,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update dut -name test-host -namespace os -pools-replace pool2',
        'shivas update machine -name chromeos-machine-1 -namespace os -zone ZONE_ATLANTA -rack chromeos1-row1-rack1',
      ]);
    });
  });

  describe('Rack editing', () => {
    const original = {
      name: 'machineLSEs/test-host',
      zone: 'ZONE_CHROMEOS1',
      rack: 'chromeos1-row1-rack1',
      machines: ['chromeos-machine-1'],
    } as unknown as MachineLSE;

    const updated = {
      name: 'machineLSEs/test-host',
      zone: 'ZONE_CHROMEOS1',
      rack: 'chromeos1-row1-rack2',
      machines: ['chromeos-machine-1'],
    } as unknown as MachineLSE;

    it('translates diff to DeviceConfigEdits format', () => {
      const { edits, paths } = translateDiffToEdits(original, updated);
      expect(edits.rack).toBe('chromeos1-row1-rack2');
      expect(paths).toEqual(['rack']);
    });

    it('generates shivas update machine command with -rack flag when only rack is updated', () => {
      const commands = generateShivasCommands(
        original,
        updated,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update machine -name chromeos-machine-1 -namespace os -rack chromeos1-row1-rack2',
      ]);
    });

    it('generates shivas update machine command with -rack - when rack is cleared', () => {
      const cleared = {
        name: 'machineLSEs/test-host',
        zone: 'ZONE_CHROMEOS1',
        rack: '',
        machines: ['chromeos-machine-1'],
      } as unknown as MachineLSE;
      const commands = generateShivasCommands(
        original,
        cleared,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update machine -name chromeos-machine-1 -namespace os -rack -',
      ]);
    });

    it('generates shivas update machine command with both updated -zone and -rack flags when both change', () => {
      const updatedBoth = {
        name: 'machineLSEs/test-host',
        zone: 'ZONE_ATLANTA',
        rack: 'chromeos1-row1-rack2',
        machines: ['chromeos-machine-1'],
      } as unknown as MachineLSE;
      const commands = generateShivasCommands(
        original,
        updatedBoth,
        'test-host',
        'os',
      );
      expect(commands).toEqual([
        'shivas update machine -name chromeos-machine-1 -namespace os -zone ZONE_ATLANTA -rack chromeos1-row1-rack2',
      ]);
    });
  });
});
