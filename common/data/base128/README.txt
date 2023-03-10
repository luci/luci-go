base128 is an encoding scheme that encoded a sequence of bytes in the low
7 bits of each byte of an output sequence of bytes.

As of 2023 Q1, it has no users, and has thus been deleted.

Prior to being deleted though, a longstanding bug in base128 was fixed, so
the most recent commit that contains the base128 library is safe to restore
if the need arises.
