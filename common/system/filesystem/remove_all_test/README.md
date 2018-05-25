tl;dr `./test.sh`

This test reproduces bug in filesystem.RemoveAll which couldn't remove path
containing read-only root-owned file. `rm -rf path` on Linux or Mac.
Unfortunately, reproduction requires `sudo chown`.

See also https://crbug.com/846378#c9
