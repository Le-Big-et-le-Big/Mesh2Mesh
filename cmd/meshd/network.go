package main

import "strconv"

// setupInterface gives mesh0, which WireGuard just attached to, addr and only
// addr: an address left from a previous tenant would still answer locally.
func setupInterface(addr string) error {
	if err := execCmd("ip", "-4", "addr", "flush", "dev", meshIface); err != nil {
		return err
	}
	if err := execCmd("ip", "addr", "add", addr, "dev", meshIface); err != nil {
		return err
	}

	if err := execCmd("ip", "link", "set", "dev", meshIface, "mtu", strconv.Itoa(meshMTU)); err != nil {
		return err
	}
	return execCmd("ip", "link", "set", "dev", meshIface, "up")
}
