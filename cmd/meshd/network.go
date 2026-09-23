package main

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
)

func bindUDP(port int) (*net.UDPConn, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return nil, fmt.Errorf("listen udp on port %d: %w", port, err)
	}
	return conn, nil
}

// setupInterface creates mesh0 if it does not exist and gives it addr.
func setupInterface(addr string) error {
	if err := exec.Command("ip", "link", "show", meshIface).Run(); err != nil {
		if err := execCmd("ip", "tuntap", "add", "dev", meshIface, "mode", "tun"); err != nil {
			return err
		}
	}

	if err := execCmd("ip", "addr", "replace", addr, "dev", meshIface); err != nil {
		return err
	}

	if err := execCmd("ip", "link", "set", "dev", meshIface, "mtu", strconv.Itoa(meshMTU)); err != nil {
		return err
	}
	return execCmd("ip", "link", "set", "dev", meshIface, "up")
}
