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

export type DeviceKind = 'chromebook' | 'pixel' | 'server';

export type StarRating = 0 | 1 | 2 | 3;

export interface Rect {
  readonly x: number;
  readonly y: number;
  readonly w: number;
  readonly h: number;
}

export interface PlatformRect extends Rect {
  /** Pass-through from below; press Down (S / ArrowDown) to drop through. */
  readonly oneWay?: boolean;
}

export interface Vec2 {
  x: number;
  y: number;
}

export interface DeviceSpawn {
  readonly id: string;
  readonly x: number;
  readonly y: number;
  readonly kind: DeviceKind;
  /** Shielded devices deflect shurikens and must be repaired with a close-range slash. */
  readonly shielded?: boolean;
}

export interface WorldHint {
  readonly x: number;
  readonly y: number;
  readonly text: string;
}

export interface StarThresholds {
  /** 5th-tier Gemini TAS (tool-assisted optimal) benchmark (ms). */
  readonly tasTimeMs: number;
  /** Hidden 4th-tier developer speedrun target (ms). */
  readonly devTimeMs: number;
  /** Max clear time (ms) for 3 stars. */
  readonly threeStarMs: number;
  /** Max clear time (ms) for 2 stars. */
  readonly twoStarMs: number;
  /** Max clear time (ms) for 1 star (always <= 10,000 ms). */
  readonly oneStarMs: number;
}

export interface LevelDef {
  readonly id: string;
  readonly index: number;
  readonly name: string;
  readonly subtitle: string;
  readonly spawn: Readonly<Vec2>;
  readonly platforms: readonly PlatformRect[];
  readonly hazards: readonly Rect[];
  readonly devices: readonly DeviceSpawn[];
  readonly hints: readonly WorldHint[];
  readonly stars: StarThresholds;
}

export interface PlayerState {
  x: number;
  y: number;
  w: number;
  h: number;
  vx: number;
  vy: number;
  facing: 1 | -1;
  onGround: boolean;
  onOneWay: boolean;
  wallDir: -1 | 0 | 1;
  airJumpsLeft: number;
  coyoteTimer: number;
  jumpBufferTimer: number;
  slashBufferTimer: number;
  shurikenBufferTimer: number;
  dropThroughTimer: number;
  slashTimer: number;
  slashCooldown: number;
  slashFacing: 1 | -1;
  shurikenCooldown: number;
  scaleX: number;
  scaleY: number;
}

export interface DeviceState extends DeviceSpawn {
  repaired: boolean;
  repairFlash: number;
  bobOffset: number;
}

export interface Shuriken {
  x: number;
  y: number;
  vx: number;
  rotation: number;
  life: number;
  pierceCount: number;
}

export interface Particle {
  x: number;
  y: number;
  vx: number;
  vy: number;
  size: number;
  color: string;
  life: number;
  maxLife: number;
  gravity: number;
}

export interface Shockwave {
  x: number;
  y: number;
  radius: number;
  maxRadius: number;
  color: string;
  life: number;
  maxLife: number;
}

export interface FloatingCallout {
  x: number;
  y: number;
  text: string;
  color: string;
  life: number;
  maxLife: number;
}

export interface GhostFrame {
  readonly t: number;
  readonly x: number;
  readonly y: number;
  readonly facing: 1 | -1;
  readonly slashing: boolean;
}

export interface StageRecord {
  readonly bestTimeMs: number;
  readonly stars: StarRating;
  readonly devMedal: boolean;
  readonly tasMedal: boolean;
  readonly splitTimesMs: readonly number[];
  readonly ghost: readonly GhostFrame[];
}

export type RunPhase = 'ready' | 'running' | 'cleared' | 'failed';

export interface InputSnapshot {
  left: boolean;
  right: boolean;
  downHeld: boolean;
  downPressed: boolean;
  jumpHeld: boolean;
  jumpPressed: boolean;
  slashPressed: boolean;
  shurikenPressed: boolean;
}

export interface SoundEvents {
  jump: boolean;
  doubleJump: boolean;
  wallJump: boolean;
  slash: boolean;
  shurikenThrow: boolean;
  shurikenEmpty: boolean;
  shieldDeflect: boolean;
  repairCount: number;
  countdownTick: boolean;
  stageClear: boolean;
  timeUp: boolean;
  hazardReset: boolean;
}

export interface LiveSplitBadge {
  deltaMs: number;
  timer: number;
}

export interface GameState {
  level: LevelDef;
  phase: RunPhase;
  elapsedMs: number;
  remainingMs: number;
  lastTickSecond: number;
  clockStarted: boolean;
  shurikensLeft: number;
  emptyStarFlashTimer: number;
  hazardFlashTimer: number;
  attemptCount: number;
  player: PlayerState;
  devices: DeviceState[];
  shurikens: Shuriken[];
  particles: Particle[];
  shockwaves: Shockwave[];
  callouts: FloatingCallout[];
  hitstopTimer: number;
  shakeTimer: number;
  shakeIntensity: number;
  recording: GhostFrame[];
  lastGhostSampleMs: number;
  splitTimesMs: number[];
  liveSplit: LiveSplitBadge | null;
  comboCount: number;
  comboTimer: number;
}
