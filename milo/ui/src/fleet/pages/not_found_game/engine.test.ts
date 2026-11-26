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

import { fleetColors } from '@/fleet/theme/tokens';

import {
  computeStars,
  createGameState,
  formatCountdown,
  formatRunTime,
  isDevTime,
  isTasTime,
  MAX_RUN_SPEED,
  sampleGhostFrame,
  stepGame,
} from './engine';
import { LEVELS, MAX_SHURIKENS, TIME_LIMIT_MS } from './levels';
import { resolveCanvasColor } from './renderer';
import { InputSnapshot } from './types';

function idleInput(overrides: Partial<InputSnapshot> = {}): InputSnapshot {
  return {
    left: false,
    right: false,
    downHeld: false,
    downPressed: false,
    jumpHeld: false,
    jumpPressed: false,
    slashPressed: false,
    shurikenPressed: false,
    ...overrides,
  };
}

describe('10-Second Bass speedrun engine', () => {
  it('keeps the 10-second countdown frozen until the first player input', () => {
    const state = createGameState(LEVELS[0]);
    stepGame(state, idleInput(), 1 / 60);
    stepGame(state, idleInput(), 1 / 60);

    expect(state.clockStarted).toBe(false);
    expect(state.remainingMs).toBe(TIME_LIMIT_MS);
    expect(state.phase).toBe('ready');

    stepGame(state, idleInput({ right: true }), 1 / 60);
    expect(state.clockStarted).toBe(true);
    expect(state.phase).toBe('running');
    expect(state.remainingMs).toBeLessThan(TIME_LIMIT_MS);
  });

  it('supports ground jump and one mid-air double-jump', () => {
    const state = createGameState(LEVELS[0]);
    for (let i = 0; i < 10; i++) {
      stepGame(state, idleInput(), 1 / 60);
    }
    expect(state.player.onGround).toBe(true);

    const jumpEv = stepGame(
      state,
      idleInput({ jumpPressed: true, jumpHeld: true }),
      1 / 60,
    );
    expect(jumpEv.jump).toBe(true);
    expect(state.player.onGround).toBe(false);
    expect(state.player.airJumpsLeft).toBe(1);

    const djEv = stepGame(
      state,
      idleInput({ jumpPressed: true, jumpHeld: true }),
      1 / 60,
    );
    expect(djEv.doubleJump).toBe(true);
    expect(state.player.airJumpsLeft).toBe(0);
    expect(state.player.vy).toBeLessThan(-480);

    const noEv = stepGame(
      state,
      idleInput({ jumpPressed: true, jumpHeld: true }),
      1 / 60,
    );
    expect(noEv.doubleJump).toBe(false);
  });

  it('buffers jump/slash/shuriken inputs during hitstop instead of dropping them', () => {
    const state = createGameState(LEVELS[0]);
    for (let i = 0; i < 10; i++) {
      stepGame(state, idleInput(), 1 / 60);
    }
    // Simulate active slash hitstop (e.g. 36ms)
    state.hitstopTimer = 0.036;

    // Player taps jump during hitstop
    const duringHitstop = stepGame(
      state,
      idleInput({ jumpPressed: true, jumpHeld: true }),
      1 / 60,
    );
    expect(duringHitstop.jump).toBe(false);
    expect(state.player.jumpBufferTimer).toBeGreaterThan(0);

    // Next frames clear hitstop -> buffered jump MUST fire!
    let jumped = false;
    for (let i = 0; i < 5; i++) {
      const ev = stepGame(state, idleInput({ jumpHeld: true }), 1 / 60);
      if (ev.jump) {
        jumped = true;
        break;
      }
    }
    expect(jumped).toBe(true);
  });

  it('drops through explicit one-way shelves on Down press', () => {
    const state = createGameState(LEVELS[0]);
    const shelf = LEVELS[0].platforms.find((pl) => pl.oneWay);
    expect(shelf).toBeDefined();
    if (!shelf) return;

    state.player.x = shelf.x + 20;
    state.player.y = shelf.y - state.player.h - 2;
    state.player.vy = 100;
    for (let i = 0; i < 5; i++) {
      stepGame(state, idleInput(), 1 / 60);
    }
    expect(state.player.onGround).toBe(true);
    expect(state.player.onOneWay).toBe(true);

    // Press Down to drop through
    stepGame(state, idleInput({ downPressed: true, downHeld: true }), 1 / 60);
    for (let i = 0; i < 6; i++) {
      stepGame(state, idleInput({ downHeld: true }), 1 / 60);
    }
    expect(state.player.y).toBeGreaterThan(shelf.y + shelf.h);
  });

  it('passes straight through one-way shelves when holding Down while falling', () => {
    const state = createGameState(LEVELS[0]);
    const shelf = LEVELS[0].platforms.find((pl) => pl.oneWay);
    expect(shelf).toBeDefined();
    if (!shelf) return;

    state.player.x = shelf.x + 20;
    state.player.y = shelf.y - state.player.h - 4;
    state.player.vy = 180;
    for (let i = 0; i < 8; i++) {
      stepGame(state, idleInput({ downHeld: true }), 1 / 60);
    }
    expect(state.player.onGround).toBe(false);
    expect(state.player.y).toBeGreaterThan(shelf.y + shelf.h);
  });

  it('repairs nearby devices with Manual Repair without lunging forward or refreshing air jump', () => {
    const state = createGameState(LEVELS[0]);
    const target = state.devices[1];
    state.player.x = target.x - 36;
    state.player.y = target.y - 14;
    state.player.vx = MAX_RUN_SPEED;
    state.player.onGround = false;
    state.player.airJumpsLeft = 0;
    state.player.facing = 1;

    const ev = stepGame(
      state,
      idleInput({ right: true, slashPressed: true }),
      1 / 60,
    );
    expect(ev.slash).toBe(true);
    expect(state.player.vx).toBeLessThanOrEqual(MAX_RUN_SPEED);
    expect(target.repaired).toBe(true);
    expect(state.player.airJumpsLeft).toBe(0);
  });

  it('hits devices with the upper part of the Manual Repair arc when attacking from below', () => {
    const state = createGameState(LEVELS[0]);
    const aboveTarget = state.devices[2]; // perched on center shelf
    // Position player below the shelf so only the upper half of the 44px arc reaches the device
    state.player.x = aboveTarget.x - 40;
    state.player.y = aboveTarget.y + 42;
    state.player.facing = 1;

    const ev = stepGame(state, idleInput({ slashPressed: true }), 1 / 60);
    expect(ev.slash).toBe(true);
    expect(aboveTarget.repaired).toBe(true);
  });

  it('limits shurikens to 3 per stage, pierces aligned standard devices, and clicks empty at 0', () => {
    const state = createGameState(LEVELS[0]);
    const first = state.devices[3];
    const second = state.devices[4];
    state.player.x = first.x - 100;
    state.player.y = first.y - 14;
    state.player.facing = 1;

    expect(state.shurikensLeft).toBe(MAX_SHURIKENS);

    const throwEv = stepGame(
      state,
      idleInput({ shurikenPressed: true }),
      1 / 60,
    );
    expect(throwEv.shurikenThrow).toBe(true);
    expect(state.shurikensLeft).toBe(MAX_SHURIKENS - 1);

    let totalRepairs = 0;
    for (let i = 0; i < 45; i++) {
      const ev = stepGame(state, idleInput(), 1 / 60);
      totalRepairs += ev.repairCount;
    }
    expect(totalRepairs).toBe(2);
    expect(first.repaired).toBe(true);
    expect(second.repaired).toBe(true);

    // Exhaust remaining stars and verify empty feedback
    state.shurikensLeft = 0;
    state.player.shurikenCooldown = 0;
    const emptyEv = stepGame(
      state,
      idleInput({ shurikenPressed: true }),
      1 / 60,
    );
    expect(emptyEv.shurikenEmpty).toBe(true);
    expect(state.emptyStarFlashTimer).toBeGreaterThan(0);
  });

  it('deflects shurikens off shielded devices but repairs them with melee slash', () => {
    const state = createGameState(LEVELS[1]);
    const shieldedDev = state.devices.find((d) => d.shielded);
    expect(shieldedDev).toBeDefined();
    if (!shieldedDev) return;

    state.player.x = shieldedDev.x - 80;
    state.player.y = shieldedDev.y - 14;
    state.player.facing = 1;

    stepGame(state, idleInput({ shurikenPressed: true }), 1 / 60);
    let deflected = false;
    for (let i = 0; i < 20; i++) {
      const ev = stepGame(state, idleInput(), 1 / 60);
      if (ev.shieldDeflect) {
        deflected = true;
        break;
      }
    }
    expect(deflected).toBe(true);
    expect(shieldedDev.repaired).toBe(false);

    state.player.x = shieldedDev.x - 36;
    state.player.y = shieldedDev.y - 14;
    const slashEv = stepGame(state, idleInput({ slashPressed: true }), 1 / 60);
    expect(slashEv.slash).toBe(true);
    expect(shieldedDev.repaired).toBe(true);
  });

  it('clears the stage immediately, tracks live splits, and computes stars and dev time', () => {
    const state = createGameState(LEVELS[0]);
    for (let i = 0; i < state.devices.length - 1; i++) {
      state.devices[i].repaired = true;
    }
    const lastDev = state.devices[state.devices.length - 1];
    state.player.x = lastDev.x - 32;
    state.player.y = lastDev.y - 14;
    state.player.facing = 1;
    state.elapsedMs = 2100;
    state.clockStarted = true;
    state.phase = 'running';

    const ev = stepGame(state, idleInput({ slashPressed: true }), 1 / 60, [
      2400,
    ]);
    expect(ev.stageClear).toBe(true);
    expect(state.phase).toBe('cleared');
    expect(state.liveSplit).not.toBeNull();
    expect(state.liveSplit?.deltaMs).toBeLessThan(0);

    expect(isTasTime(LEVELS[0], 1167)).toBe(true);
    expect(isTasTime(LEVELS[0], 1200)).toBe(false);
    expect(isDevTime(LEVELS[0], 2017)).toBe(true);
    expect(isDevTime(LEVELS[0], 2100)).toBe(false);
    expect(computeStars(LEVELS[0], 2500)).toBe(3);
    expect(computeStars(LEVELS[0], 4200)).toBe(2);
    expect(computeStars(LEVELS[0], 8500)).toBe(1);
    expect(computeStars(LEVELS[0], 10_500)).toBe(0);
    expect(formatCountdown(9450)).toBe('09.45');
    expect(formatRunTime(2420)).toBe('02.420');
  });

  it('fails the run when the 10-second countdown reaches 0', () => {
    const state = createGameState(LEVELS[0]);
    state.clockStarted = true;
    state.phase = 'running';
    state.elapsedMs = 9990;
    state.remainingMs = 10;

    const ev = stepGame(state, idleInput(), 1 / 60);
    expect(ev.timeUp).toBe(true);
    expect(state.phase).toBe('failed');
    expect(state.remainingMs).toBe(0);
  });

  it('instantly resets with a red flash and incremented attempt counter when touching a laser', () => {
    const state = createGameState(LEVELS[3]);
    const hazard = LEVELS[3].hazards[0];
    state.clockStarted = true;
    state.elapsedMs = 2500;
    state.player.x = hazard.x + 10;
    state.player.y = hazard.y - 10;

    const ev = stepGame(state, idleInput(), 1 / 60);
    expect(ev.hazardReset).toBe(true);
    expect(state.attemptCount).toBe(2);
    expect(state.hazardFlashTimer).toBeGreaterThan(0);
    expect(state.elapsedMs).toBe(0);
    expect(state.shurikensLeft).toBe(MAX_SHURIKENS);
    expect(state.phase).toBe('ready');
  });

  it('interpolates ghost frames smoothly between recorded timestamps', () => {
    const ghost = [
      { t: 0, x: 10, y: 20, facing: 1 as const, slashing: false },
      { t: 100, x: 110, y: 120, facing: -1 as const, slashing: true },
    ];
    const mid = sampleGhostFrame(ghost, 50);
    expect(mid).toEqual({ t: 50, x: 60, y: 70, facing: 1, slashing: false });
  });

  it('resolves CSS-var Fleet tokens for Canvas 2D with live theme override and light fallback', () => {
    const fallback = resolveCanvasColor(
      { getPropertyValue: () => '' },
      fleetColors.surface,
    );
    expect(fallback).toBe('#ffffff');

    const darkOverride = resolveCanvasColor(
      {
        getPropertyValue: (name) => (name === '--fc-surface' ? '#131314' : ''),
      },
      fleetColors.surface,
    );
    expect(darkOverride).toBe('#131314');
  });
});
