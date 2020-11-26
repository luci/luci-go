package main

import (
	"bytes"
	"fmt"

	"go.chromium.org/luci/common/data/cmpbin"
)

func main() {
	zstdFrameMagic := []byte{0x28, 0xb5, 0x2f, 0xfd}
	r := bytes.NewBuffer(zstdFrameMagic)
	mag, n, err := cmpbin.ReadUint(r)
	if err != nil {
		panic(err) // panic: cmpbin: varint overflows
	}
	fmt.Printf("read %d in %d bytes\n", mag, n)
}
