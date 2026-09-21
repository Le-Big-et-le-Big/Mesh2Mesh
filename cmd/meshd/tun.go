//go:build linux

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	tunDevice = "/dev/net/tun"

	iffTun  = 0x0001
	iffNoPI = 0x1000

	// TUNSETIFF attaches the open /dev/net/tun handle to a named interface.
	tunSetIff = 0x400454ca

	ifNameSize = 16
)

// ifreq is the interface request struct TUNSETIFF reads.
type ifreq struct {
	name  [ifNameSize]byte
	flags uint16
	_     [22]byte
}

func openTUN(name string) (*os.File, error) {
	if len(name) >= ifNameSize {
		return nil, fmt.Errorf("open tun: interface name %q is too long (max %d bytes)", name, ifNameSize-1)
	}

	f, err := os.OpenFile(tunDevice, os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w (is the tun module loaded, and are we root?)", tunDevice, err)
	}

	req := ifreq{flags: iffTun | iffNoPI}
	copy(req.name[:], name)

	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), tunSetIff, uintptr(unsafe.Pointer(&req))); errno != 0 {
		f.Close()
		return nil, fmt.Errorf("attach to %s: %w", name, errno)
	}
	return f, nil
}
