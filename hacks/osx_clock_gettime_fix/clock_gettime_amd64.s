// Copyright 2021 The LUCI Authors.
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

//+build darwin,amd64,go1.16

#include "textflag.h"

TEXT libc_clock_gettime(SB),NOSPLIT,$0-0
    PUSHQ BP
    MOVQ SP, BP
    MOVQ SI, BX
    MOVQ SI, DI // arg 1: *timeval
    XORQ SI, SI // arg 2: *timezone (nil)
    CALL libc_gettimeofday(SB)

    // hacky: usec to nsec
    // clock_gettime expects timespec, which is { sec int64; nsec int64 }
    // gettimeofday expects timeval, which is { sec int64; usec int32 }
    MOVL 8(BX), AX
    IMULQ $1000, AX
    MOVQ AX, 8(BX)

    POPQ BP
    RET
