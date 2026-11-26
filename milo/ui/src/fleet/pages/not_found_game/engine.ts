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

import { fleetColors, statusTones } from '@/fleet/theme/tokens';

import {
  MAX_SHURIKENS,
  TIME_LIMIT_MS,
  WORLD_HEIGHT,
  WORLD_WIDTH,
} from './levels';
import {
  GameState,
  GhostFrame,
  InputSnapshot,
  LevelDef,
  Rect,
  SoundEvents,
  StarRating,
} from './types';

export const PLAYER_SIZE = 28;
const GRAVITY_RISE = 1720;
const GRAVITY_FALL = 2320;
const GRAVITY_FAST_FALL = 3400;
const MAX_FALL_SPEED = 940;
const MAX_FAST_FALL_SPEED = 1120;
export const MAX_RUN_SPEED = 385;
const GROUND_ACCEL = 3400;
const AIR_ACCEL = 2400;
const GROUND_FRICTION = 3200;
const AIR_DRAG = 520;
const JUMP_VELOCITY = -600;
const DOUBLE_JUMP_VELOCITY = -560;
const WALL_SLIDE_MAX_VY = 140;
const WALL_JUMP_VX = 415;
const WALL_JUMP_VY = -565;
const COYOTE_TIME = 0.1;
const INPUT_BUFFER_TIME = 0.11;
const DROP_THROUGH_TIME = 0.16;

const SLASH_DURATION = 0.13;
const SLASH_COOLDOWN = 0.2;
// Matches the rendered 44px-radius crescent arc (including outer glow)
const SLASH_REACH_X = 72;
const SLASH_HEIGHT = 104;

const SHURIKEN_SPEED = 1100;
const SHURIKEN_COOLDOWN = 0.14;
const SHURIKEN_HIT_DX = 26;
const SHURIKEN_HIT_DY = 26;

const GHOST_SAMPLE_INTERVAL_MS = 33;

export function createEmptySoundEvents(): SoundEvents {
  return {
    jump: false,
    doubleJump: false,
    wallJump: false,
    slash: false,
    shurikenThrow: false,
    shurikenEmpty: false,
    shieldDeflect: false,
    repairCount: 0,
    countdownTick: false,
    stageClear: false,
    timeUp: false,
    hazardReset: false,
  };
}

export function computeStars(level: LevelDef, elapsedMs: number): StarRating {
  if (elapsedMs <= level.stars.threeStarMs) return 3;
  if (elapsedMs <= level.stars.twoStarMs) return 2;
  if (elapsedMs <= level.stars.oneStarMs) return 1;
  return 0;
}

export function isDevTime(level: LevelDef, elapsedMs: number): boolean {
  return elapsedMs <= level.stars.devTimeMs;
}

export function isTasTime(level: LevelDef, elapsedMs: number): boolean {
  return elapsedMs <= level.stars.tasTimeMs;
}

export function formatCountdown(ms: number): string {
  const clamped = Math.max(0, ms);
  const seconds = Math.floor(clamped / 1000);
  const centis = Math.floor((clamped % 1000) / 10);
  return `${String(seconds).padStart(2, '0')}.${String(centis).padStart(2, '0')}`;
}

export function formatRunTime(ms: number): string {
  const clamped = Math.max(0, ms);
  const seconds = Math.floor(clamped / 1000);
  const millis = Math.floor(clamped % 1000);
  return `${String(seconds).padStart(2, '0')}.${String(millis).padStart(3, '0')}`;
}

export function createGameState(
  level: LevelDef,
  attemptCount = 1,
  hazardFlashTimer = 0,
): GameState {
  return {
    level,
    phase: 'ready',
    elapsedMs: 0,
    remainingMs: TIME_LIMIT_MS,
    lastTickSecond: 4,
    clockStarted: false,
    shurikensLeft: MAX_SHURIKENS,
    emptyStarFlashTimer: 0,
    hazardFlashTimer,
    attemptCount,
    player: {
      x: level.spawn.x,
      y: level.spawn.y,
      w: PLAYER_SIZE,
      h: PLAYER_SIZE,
      vx: 0,
      vy: 0,
      facing: 1,
      onGround: false,
      onOneWay: false,
      wallDir: 0,
      airJumpsLeft: 1,
      coyoteTimer: 0,
      jumpBufferTimer: 0,
      slashBufferTimer: 0,
      shurikenBufferTimer: 0,
      dropThroughTimer: 0,
      slashTimer: 0,
      slashCooldown: 0,
      slashFacing: 1,
      shurikenCooldown: 0,
      scaleX: 1,
      scaleY: 1,
    },
    devices: level.devices.map((d, idx) => ({
      ...d,
      repaired: false,
      repairFlash: 0,
      bobOffset: idx * 1.15,
    })),
    shurikens: [],
    particles: [],
    shockwaves: [],
    callouts: [],
    hitstopTimer: 0,
    shakeTimer: 0,
    shakeIntensity: 0,
    recording: [
      {
        t: 0,
        x: level.spawn.x,
        y: level.spawn.y,
        facing: 1,
        slashing: false,
      },
    ],
    lastGhostSampleMs: 0,
    splitTimesMs: [],
    liveSplit: null,
    comboCount: 0,
    comboTimer: 0,
  };
}

function rectsOverlap(a: Rect, b: Rect): boolean {
  return (
    a.x < b.x + b.w && a.x + a.w > b.x && a.y < b.y + b.h && a.y + a.h > b.y
  );
}

function spawnParticles(
  state: GameState,
  x: number,
  y: number,
  color: string,
  count: number,
  speedMin: number,
  speedMax: number,
  gravity = 420,
  sizeMin = 3,
  sizeMax = 6,
): void {
  for (let i = 0; i < count; i++) {
    const angle = (Math.PI * 2 * i) / count + (Math.random() - 0.5) * 0.55;
    const speed = speedMin + Math.random() * (speedMax - speedMin);
    const maxLife = 0.22 + Math.random() * 0.28;
    state.particles.push({
      x,
      y,
      vx: Math.cos(angle) * speed,
      vy: Math.sin(angle) * speed,
      size: sizeMin + Math.random() * (sizeMax - sizeMin),
      color,
      life: maxLife,
      maxLife,
      gravity,
    });
  }
}

function addShake(state: GameState, intensity: number, duration: number): void {
  state.shakeIntensity = Math.max(state.shakeIntensity, intensity);
  state.shakeTimer = Math.max(state.shakeTimer, duration);
}

function repairDevice(
  state: GameState,
  deviceIdx: number,
  sound: SoundEvents,
  fromSlash: boolean,
  pbSplits?: readonly number[],
): boolean {
  const dev = state.devices[deviceIdx];
  if (!dev || dev.repaired) return false;

  dev.repaired = true;
  dev.repairFlash = 1;
  state.comboCount = state.comboTimer > 0 ? state.comboCount + 1 : 1;
  state.comboTimer = 1.6;
  sound.repairCount += 1;

  // Record split time & compute live delta vs Personal Best
  const splitIdx = state.splitTimesMs.length;
  const nowMs = Math.round(state.elapsedMs);
  state.splitTimesMs.push(nowMs);
  if (pbSplits && splitIdx < pbSplits.length) {
    state.liveSplit = {
      deltaMs: nowMs - pbSplits[splitIdx],
      timer: 1.35,
    };
  }

  // Hit-confirm forward burst on Manual Repair (prevents empty-air slash-mashing speed exploit!)
  spawnParticles(
    state,
    dev.x,
    dev.y,
    statusTones.success.dot,
    16,
    100,
    310,
    280,
    3,
    6,
  );
  spawnParticles(
    state,
    dev.x,
    dev.y,
    fleetColors.surface,
    6,
    60,
    190,
    120,
    2,
    4,
  );
  state.shockwaves.push({
    x: dev.x,
    y: dev.y,
    radius: 10,
    maxRadius: 58,
    color: fromSlash ? statusTones.info.dot : statusTones.success.dot,
    life: 0.22,
    maxLife: 0.22,
  });
  state.callouts.push({
    x: dev.x,
    y: dev.y - 20,
    text: state.comboCount > 1 ? `READY x${state.comboCount}!` : 'READY!',
    color: statusTones.success.fg,
    life: 0.68,
    maxLife: 0.68,
  });

  addShake(state, fromSlash ? 5 : 3.5, 0.09);
  if (fromSlash) {
    state.hitstopTimer = Math.max(state.hitstopTimer, 0.036);
  }

  // Classic 10 Second Ninja: repairing the last device clears the stage immediately!
  if (state.devices.every((d) => d.repaired)) {
    state.phase = 'cleared';
    sound.stageClear = true;
    state.recording.push({
      t: nowMs,
      x: Math.round(state.player.x * 10) / 10,
      y: Math.round(state.player.y * 10) / 10,
      facing: state.player.facing,
      slashing: state.player.slashTimer > 0,
    });
    addShake(state, 6.5, 0.16);
    spawnParticles(
      state,
      state.player.x + state.player.w / 2,
      state.player.y + state.player.h / 2,
      statusTones.warning.dot,
      24,
      110,
      360,
      180,
      4,
      7,
    );
  }

  return true;
}

function updateEffects(state: GameState, dt: number): void {
  for (const d of state.devices) {
    if (d.repairFlash > 0) {
      d.repairFlash = Math.max(0, d.repairFlash - dt * 3.5);
    }
  }

  for (let i = state.particles.length - 1; i >= 0; i--) {
    const pt = state.particles[i];
    pt.life -= dt;
    if (pt.life <= 0) {
      state.particles.splice(i, 1);
      continue;
    }
    pt.vy += pt.gravity * dt;
    pt.x += pt.vx * dt;
    pt.y += pt.vy * dt;
  }

  for (let i = state.shockwaves.length - 1; i >= 0; i--) {
    const sw = state.shockwaves[i];
    sw.life -= dt;
    if (sw.life <= 0) {
      state.shockwaves.splice(i, 1);
      continue;
    }
    const progress = 1 - sw.life / sw.maxLife;
    sw.radius = sw.maxRadius * (0.15 + 0.85 * (1 - (1 - progress) ** 2));
  }

  for (let i = state.callouts.length - 1; i >= 0; i--) {
    const c = state.callouts[i];
    c.life -= dt;
    c.y -= 36 * dt;
    if (c.life <= 0) {
      state.callouts.splice(i, 1);
    }
  }

  if (state.shakeTimer > 0) {
    state.shakeTimer = Math.max(0, state.shakeTimer - dt);
    if (state.shakeTimer === 0) {
      state.shakeIntensity = 0;
    }
  }

  if (state.emptyStarFlashTimer > 0) {
    state.emptyStarFlashTimer = Math.max(0, state.emptyStarFlashTimer - dt);
  }

  if (state.hazardFlashTimer > 0) {
    state.hazardFlashTimer = Math.max(0, state.hazardFlashTimer - dt);
  }

  if (state.liveSplit) {
    state.liveSplit.timer -= dt;
    if (state.liveSplit.timer <= 0) {
      state.liveSplit = null;
    }
  }

  if (state.comboTimer > 0) {
    state.comboTimer = Math.max(0, state.comboTimer - dt);
    if (state.comboTimer === 0) {
      state.comboCount = 0;
    }
  }

  state.player.scaleX += (1 - state.player.scaleX) * Math.min(1, dt * 15);
  state.player.scaleY += (1 - state.player.scaleY) * Math.min(1, dt * 15);
}

export function getSlashHitbox(state: GameState): Rect {
  const p = state.player;
  const cx = p.x + p.w / 2;
  const cy = p.y + p.h / 2;
  if (p.slashFacing > 0) {
    return {
      x: cx - 14,
      y: cy - SLASH_HEIGHT / 2,
      w: SLASH_REACH_X + 14,
      h: SLASH_HEIGHT,
    };
  }
  return {
    x: cx - SLASH_REACH_X,
    y: cy - SLASH_HEIGHT / 2,
    w: SLASH_REACH_X + 14,
    h: SLASH_HEIGHT,
  };
}

export function stepGame(
  state: GameState,
  input: InputSnapshot,
  dt: number,
  pbSplits?: readonly number[],
): SoundEvents {
  const sound = createEmptySoundEvents();
  const safeDt = Math.min(dt, 0.05);

  if (state.phase === 'cleared' || state.phase === 'failed') {
    updateEffects(state, safeDt);
    return sound;
  }

  const p = state.player;

  // Buffer discrete presses BEFORE hitstop early-return so hitstop NEVER eats inputs!
  if (input.jumpPressed) {
    p.jumpBufferTimer = INPUT_BUFFER_TIME;
  }
  if (input.slashPressed) {
    p.slashBufferTimer = INPUT_BUFFER_TIME;
  }
  if (input.shurikenPressed) {
    p.shurikenBufferTimer = INPUT_BUFFER_TIME;
  }
  if ((input.downPressed || input.downHeld) && p.onGround && p.onOneWay) {
    p.dropThroughTimer = DROP_THROUGH_TIME;
    p.onGround = false;
    p.onOneWay = false;
    p.coyoteTimer = 0;
    p.y += 4;
    p.vy = Math.max(p.vy, 190);
  }

  // Clock freezes at 10.00 until first player input
  if (
    !state.clockStarted &&
    (input.left ||
      input.right ||
      input.downPressed ||
      input.jumpPressed ||
      input.slashPressed ||
      input.shurikenPressed)
  ) {
    state.clockStarted = true;
    state.phase = 'running';
  }

  if (state.hitstopTimer > 0) {
    state.hitstopTimer = Math.max(0, state.hitstopTimer - safeDt);
    updateEffects(state, safeDt * 0.25);
    return sound;
  }

  if (state.clockStarted) {
    const prevStars = computeStars(state.level, state.elapsedMs);
    state.elapsedMs += safeDt * 1000;
    state.remainingMs = Math.max(0, TIME_LIMIT_MS - state.elapsedMs);
    const nextStars = computeStars(state.level, state.elapsedMs);
    if (nextStars < prevStars && nextStars > 0) {
      sound.countdownTick = true;
    }
    const wholeSecLeft = Math.ceil(state.remainingMs / 1000);
    if (
      wholeSecLeft <= 3 &&
      wholeSecLeft > 0 &&
      wholeSecLeft < state.lastTickSecond
    ) {
      state.lastTickSecond = wholeSecLeft;
      sound.countdownTick = true;
    }
    if (state.remainingMs <= 0) {
      state.phase = 'failed';
      sound.timeUp = true;
      return sound;
    }
  }

  // Decay input buffers only outside hitstop
  if (!input.jumpPressed) {
    p.jumpBufferTimer = Math.max(0, p.jumpBufferTimer - safeDt);
  }
  if (!input.slashPressed) {
    p.slashBufferTimer = Math.max(0, p.slashBufferTimer - safeDt);
  }
  if (!input.shurikenPressed) {
    p.shurikenBufferTimer = Math.max(0, p.shurikenBufferTimer - safeDt);
  }
  p.dropThroughTimer = Math.max(0, p.dropThroughTimer - safeDt);

  const moveDir = (input.right ? 1 : 0) - (input.left ? 1 : 0);
  if (moveDir !== 0) {
    p.facing = moveDir > 0 ? 1 : -1;
  }

  if (p.onGround) {
    p.coyoteTimer = COYOTE_TIME;
    p.airJumpsLeft = 1;
  } else {
    p.coyoteTimer = Math.max(0, p.coyoteTimer - safeDt);
  }

  // Horizontal movement
  if (moveDir !== 0) {
    const accel = p.onGround ? GROUND_ACCEL : AIR_ACCEL;
    const targetVx = moveDir * MAX_RUN_SPEED;
    if (p.vx < targetVx) {
      p.vx = Math.min(targetVx, p.vx + accel * safeDt);
    } else if (p.vx > targetVx) {
      p.vx = Math.max(targetVx, p.vx - accel * safeDt);
    }
  } else {
    const decel = (p.onGround ? GROUND_FRICTION : AIR_DRAG) * safeDt;
    if (Math.abs(p.vx) <= decel) {
      p.vx = 0;
    } else {
      p.vx -= Math.sign(p.vx) * decel;
    }
  }

  // Jump / Wall-jump / Air Double-jump
  if (p.jumpBufferTimer > 0) {
    if (p.coyoteTimer > 0) {
      p.vy = JUMP_VELOCITY;
      p.onGround = false;
      p.onOneWay = false;
      p.coyoteTimer = 0;
      p.jumpBufferTimer = 0;
      p.scaleX = 0.82;
      p.scaleY = 1.22;
      sound.jump = true;
      spawnParticles(
        state,
        p.x + p.w / 2,
        p.y + p.h,
        fleetColors.surface,
        5,
        35,
        115,
        180,
        2,
        4,
      );
    } else if (p.wallDir !== 0) {
      p.vx = -p.wallDir * WALL_JUMP_VX;
      p.vy = WALL_JUMP_VY;
      p.facing = p.wallDir > 0 ? -1 : 1;
      p.wallDir = 0;
      p.airJumpsLeft = 1;
      p.jumpBufferTimer = 0;
      p.scaleX = 0.8;
      p.scaleY = 1.24;
      sound.wallJump = true;
      spawnParticles(
        state,
        p.x + (p.facing > 0 ? 0 : p.w),
        p.y + p.h / 2,
        fleetColors.primary,
        7,
        55,
        165,
        200,
        2,
        5,
      );
    } else if (p.airJumpsLeft > 0) {
      p.vy = DOUBLE_JUMP_VELOCITY;
      p.airJumpsLeft -= 1;
      p.jumpBufferTimer = 0;
      p.scaleX = 0.78;
      p.scaleY = 1.26;
      sound.doubleJump = true;
      state.shockwaves.push({
        x: p.x + p.w / 2,
        y: p.y + p.h,
        radius: 6,
        maxRadius: 34,
        color: statusTones.info.dot,
        life: 0.16,
        maxLife: 0.16,
      });
      spawnParticles(
        state,
        p.x + p.w / 2,
        p.y + p.h,
        statusTones.info.dot,
        7,
        50,
        150,
        160,
        2,
        4,
      );
    }
  }

  // Variable jump height cut on early jump release
  if (!input.jumpHeld && p.vy < -230) {
    p.vy += GRAVITY_RISE * 1.45 * safeDt;
  }

  // Asymmetric gravity (1.35x falling) + fast-fall when holding Down
  const fastFalling = !p.onGround && input.downHeld && p.vy >= -40;
  const gravity = fastFalling
    ? GRAVITY_FAST_FALL
    : p.vy > 0
      ? GRAVITY_FALL
      : GRAVITY_RISE;
  const maxFall = fastFalling ? MAX_FAST_FALL_SPEED : MAX_FALL_SPEED;
  p.vy = Math.min(maxFall, p.vy + gravity * safeDt);
  if (
    !p.onGround &&
    !fastFalling &&
    p.wallDir !== 0 &&
    p.vy > WALL_SLIDE_MAX_VY
  ) {
    p.vy = WALL_SLIDE_MAX_VY;
  }

  // Melee Manual Repair (Z / J)
  p.slashCooldown = Math.max(0, p.slashCooldown - safeDt);
  p.slashTimer = Math.max(0, p.slashTimer - safeDt);
  if (p.slashBufferTimer > 0 && p.slashCooldown <= 0) {
    p.slashBufferTimer = 0;
    p.slashTimer = SLASH_DURATION;
    p.slashCooldown = SLASH_COOLDOWN;
    p.slashFacing = p.facing;
    p.scaleX = 1.24;
    p.scaleY = 0.82;
    sound.slash = true;
  }

  // Piercing Repair Shuriken Throw (X / K — 3 per stage)
  p.shurikenCooldown = Math.max(0, p.shurikenCooldown - safeDt);
  if (p.shurikenBufferTimer > 0 && p.shurikenCooldown <= 0) {
    p.shurikenBufferTimer = 0;
    if (state.shurikensLeft > 0) {
      state.shurikensLeft -= 1;
      p.shurikenCooldown = SHURIKEN_COOLDOWN;
      sound.shurikenThrow = true;
      const cx = p.x + p.w / 2;
      const cy = p.y + p.h / 2;
      state.shurikens.push({
        x: cx + p.facing * 16,
        y: cy,
        vx: p.facing * SHURIKEN_SPEED,
        rotation: 0,
        life: 1.1,
        pierceCount: 0,
      });
      spawnParticles(
        state,
        cx + p.facing * 16,
        cy,
        statusTones.warning.dot,
        5,
        50,
        140,
        60,
        2,
        4,
      );
    } else {
      p.shurikenCooldown = SHURIKEN_COOLDOWN;
      state.emptyStarFlashTimer = 0.28;
      sound.shurikenEmpty = true;
    }
  }

  // Explicit solid blocks vs one-way shelves
  const solids = state.level.platforms.filter((pl) => !pl.oneWay);
  const shelves = state.level.platforms.filter((pl) => Boolean(pl.oneWay));

  // Horizontal move
  p.x += p.vx * safeDt;
  p.wallDir = 0;

  if (p.x < 0) {
    p.x = 0;
    p.vx = Math.max(0, p.vx);
    p.wallDir = -1;
  } else if (p.x + p.w > WORLD_WIDTH) {
    p.x = WORLD_WIDTH - p.w;
    p.vx = Math.min(0, p.vx);
    p.wallDir = 1;
  }

  for (const s of solids) {
    if (rectsOverlap(p, s)) {
      if (p.vx > 0) {
        p.x = s.x - p.w;
        p.vx = 0;
        p.wallDir = 1;
      } else if (p.vx < 0) {
        p.x = s.x + s.w;
        p.vx = 0;
        p.wallDir = -1;
      }
    }
  }

  // Widened 6px wall adjacency probe so early wall-jump presses don't burn double-jump
  if (!p.onGround && p.wallDir === 0) {
    const probeLeft: Rect = { x: p.x - 6, y: p.y + 4, w: 6, h: p.h - 8 };
    const probeRight: Rect = { x: p.x + p.w, y: p.y + 4, w: 6, h: p.h - 8 };
    for (const s of solids) {
      if (rectsOverlap(probeLeft, s)) {
        p.wallDir = -1;
        break;
      }
      if (rectsOverlap(probeRight, s)) {
        p.wallDir = 1;
        break;
      }
    }
  }

  // Vertical move
  const wasOnGround = p.onGround;
  const prevBottom = p.y + p.h;
  p.y += p.vy * safeDt;
  p.onGround = false;
  p.onOneWay = false;

  if (p.y < 0) {
    p.y = 0;
    if (p.vy < 0) p.vy = 0;
  }

  const landOn = (surfaceY: number, isOneWay: boolean) => {
    p.y = surfaceY - p.h;
    p.vy = 0;
    p.onGround = true;
    p.onOneWay = isOneWay;
    p.airJumpsLeft = 1;
    if (!wasOnGround) {
      p.scaleX = 1.22;
      p.scaleY = 0.8;
    }
  };

  for (const s of solids) {
    if (rectsOverlap(p, s)) {
      if (p.vy >= 0) {
        landOn(s.y, false);
      } else if (p.vy < 0) {
        p.y = s.y + s.h;
        p.vy = 0;
      }
    }
  }

  if (p.vy >= 0 && !input.downHeld && p.dropThroughTimer <= 0) {
    for (const shelf of shelves) {
      if (prevBottom <= shelf.y + 6 && rectsOverlap(p, shelf)) {
        landOn(shelf.y, true);
      }
    }
  }

  // Check red hazard lasers or falling off screen -> instant reset with red flash & attempt increment!
  const hitHazard =
    p.y > WORLD_HEIGHT + 40 ||
    state.level.hazards.some((h) => rectsOverlap(p, h));
  if (hitHazard) {
    const fresh = createGameState(state.level, state.attemptCount + 1, 0.24);
    Object.assign(state, fresh);
    sound.hazardReset = true;
    return sound;
  }

  // Active melee slash hits BOTH standard and shielded devices
  if (p.slashTimer > 0) {
    const slashBox = getSlashHitbox(state);
    for (let i = 0; i < state.devices.length; i++) {
      const dev = state.devices[i];
      if (dev.repaired) continue;
      const devBox: Rect = { x: dev.x - 18, y: dev.y - 18, w: 36, h: 36 };
      if (rectsOverlap(slashBox, devBox)) {
        repairDevice(state, i, sound, true, pbSplits);
        if (sound.stageClear) break;
      }
    }
  }

  // Update flying piercing shurikens
  for (let i = state.shurikens.length - 1; i >= 0; i--) {
    const star = state.shurikens[i];
    star.life -= safeDt;
    star.x += star.vx * safeDt;
    star.rotation += Math.sign(star.vx) * 22 * safeDt;

    if (star.life <= 0 || star.x < -20 || star.x > WORLD_WIDTH + 20) {
      state.shurikens.splice(i, 1);
      continue;
    }

    // Pierces standard devices, deflects/stops on shielded devices
    let consumed = false;
    for (let dIdx = 0; dIdx < state.devices.length; dIdx++) {
      const dev = state.devices[dIdx];
      if (dev.repaired) continue;
      if (
        Math.abs(dev.x - star.x) <= SHURIKEN_HIT_DX &&
        Math.abs(dev.y - star.y) <= SHURIKEN_HIT_DY
      ) {
        if (dev.shielded) {
          sound.shieldDeflect = true;
          spawnParticles(
            state,
            star.x,
            star.y,
            statusTones.warning.dot,
            10,
            80,
            240,
            280,
            2,
            5,
          );
          state.callouts.push({
            x: dev.x,
            y: dev.y - 22,
            text: 'NEEDS MANUAL!',
            color: statusTones.warning.fg,
            life: 0.72,
            maxLife: 0.72,
          });
          consumed = true;
          break;
        } else {
          repairDevice(state, dIdx, sound, false, pbSplits);
          star.pierceCount += 1;
          if (star.pierceCount >= 2) {
            state.hitstopTimer = Math.max(state.hitstopTimer, 0.028);
            addShake(state, 5, 0.1);
            spawnParticles(
              state,
              dev.x,
              dev.y,
              statusTones.warning.dot,
              12,
              90,
              280,
              200,
              3,
              6,
            );
            state.callouts.push({
              x: dev.x,
              y: dev.y - 36,
              text:
                star.pierceCount === 2
                  ? 'AUTO-REPAIR x2!'
                  : `AUTO-REPAIR x${star.pierceCount}!`,
              color: statusTones.warning.fg,
              life: 0.8,
              maxLife: 0.8,
            });
          }
          if (sound.stageClear) break;
        }
      }
    }

    if (sound.stageClear) break;
    if (consumed) {
      state.shurikens.splice(i, 1);
      continue;
    }

    // Solid walls stop shurikens; one-way shelves let them pass cleanly
    const starBox: Rect = { x: star.x - 5, y: star.y - 5, w: 10, h: 10 };
    for (const s of solids) {
      if (rectsOverlap(starBox, s)) {
        spawnParticles(
          state,
          star.x,
          star.y,
          statusTones.warning.dot,
          6,
          50,
          150,
          240,
          2,
          4,
        );
        consumed = true;
        break;
      }
    }
    if (consumed) {
      state.shurikens.splice(i, 1);
    }
  }

  // Record PB ghost sample
  if (
    state.clockStarted &&
    state.phase === 'running' &&
    state.elapsedMs - state.lastGhostSampleMs >= GHOST_SAMPLE_INTERVAL_MS
  ) {
    state.lastGhostSampleMs = state.elapsedMs;
    state.recording.push({
      t: Math.round(state.elapsedMs),
      x: Math.round(p.x * 10) / 10,
      y: Math.round(p.y * 10) / 10,
      facing: p.facing,
      slashing: p.slashTimer > 0,
    });
  }

  updateEffects(state, safeDt);
  return sound;
}

export function sampleGhostFrame(
  ghost: readonly GhostFrame[],
  elapsedMs: number,
): GhostFrame | null {
  if (ghost.length === 0) return null;
  if (elapsedMs <= ghost[0].t) return ghost[0];
  const last = ghost[ghost.length - 1];
  if (elapsedMs >= last.t) return last;

  let lo = 0;
  let hi = ghost.length - 1;
  while (lo + 1 < hi) {
    const mid = (lo + hi) >> 1;
    if (ghost[mid].t <= elapsedMs) {
      lo = mid;
    } else {
      hi = mid;
    }
  }
  const a = ghost[lo];
  const b = ghost[hi];
  const span = Math.max(1, b.t - a.t);
  const ratio = (elapsedMs - a.t) / span;
  return {
    t: elapsedMs,
    x: a.x + (b.x - a.x) * ratio,
    y: a.y + (b.y - a.y) * ratio,
    facing: a.facing,
    slashing: a.slashing,
  };
}
