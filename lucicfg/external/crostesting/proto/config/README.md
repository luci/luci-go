# crostesting protos

These protos are used by the Chrome OS testing infrastructure. They live here in
luci-go's repo so that they can be compiled into the lucicfg Skylark
interpreter package.

To regenerate the .pb.go files, please cd to src/go.chromium.org/luci/lucicfg
and run

```shell
go run github.com/luci/luci-go/grpc/cmd/cproto external/crostesting/proto/config/
```
