package ipc

import (
	"bytes"
	"syscall"
)

func syscallUmask(mask int) int {
	return syscall.Umask(mask)
}

func bytesTrimRight(data []byte, cut byte) []byte {
	return bytes.TrimRight(data, string([]byte{cut}))
}
