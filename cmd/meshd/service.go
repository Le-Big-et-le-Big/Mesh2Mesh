package main

import (
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	serviceName = "mesh2mesh.service"
	unitPath    = "/etc/systemd/system/" + serviceName
	pidPath     = "/run/meshd.pid"

	// stopTimeout is how long `meshd stop` waits for a foreground meshd to exit.
	stopTimeout = 5 * time.Second
)

const unitTemplate = `[Unit]
Description=Mesh2Mesh node
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
`

func hasSystemd() bool {
	_, err := os.Stat("/run/systemd/system")
	return err == nil
}

// Writes the unit for this binary and starts or restarts it, so the
// service picks up whatever setup, join or run just wrote to the state file.
func startService(statePath string, verbose bool, st *state, prefix netip.Prefix) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate the meshd binary: %w", err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fmt.Errorf("locate the meshd binary: %w", err)
	}

	cmdline := []string{exe, "run", "--foreground"}
	if statePath != defaultStatePath {
		abs, err := filepath.Abs(statePath)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", statePath, err)
		}
		cmdline = append(cmdline, "--state", abs)
	}
	if verbose {
		cmdline = append(cmdline, "-v")
	}

	unit := fmt.Sprintf(unitTemplate, strings.Join(cmdline, " "))
	if err := os.WriteFile(unitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write %s: %w (run with sudo?)", unitPath, err)
	}
	for _, args := range [][]string{
		{"daemon-reload"},
		{"enable", "--quiet", serviceName},
		{"restart", serviceName},
	} {
		if err := execCmd("systemctl", args...); err != nil {
			return err
		}
	}

	// Give it a moment to fail on a bad state file or a taken port.
	time.Sleep(time.Second)
	if !serviceActive() {
		logs, _ := exec.Command("journalctl", "-u", serviceName, "-n", "10", "--no-pager", "-o", "cat").Output()
		return fmt.Errorf("%s did not stay up:\n%s", serviceName, strings.TrimSpace(string(logs)))
	}

	fmt.Printf("meshd is running in tenant %q as %s\n  logs: journalctl -u %s -f\n  stop: sudo meshd stop\n",
		st.TenantName, prefix, serviceName)
	return nil
}

func stopCmd(args []string) error {
	fset := flag.NewFlagSet("stop", flag.ContinueOnError)
	if err := fset.Parse(args); err != nil {
		return err
	}
	if fset.NArg() > 0 {
		return fmt.Errorf("stop: unexpected argument %q", fset.Arg(0))
	}
	if os.Geteuid() != 0 {
		return errors.New("stop: must run as root (sudo meshd stop)")
	}

	stopped := false

	if _, err := os.Stat(unitPath); err == nil && hasSystemd() {
		stopped = serviceActive()
		// Disable too, or the service comes back on the next boot.
		if err := execCmd("systemctl", "disable", "--now", "--quiet", serviceName); err != nil {
			return err
		}
	}

	if pid, ok := runningPID(); ok {
		if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("signal meshd (pid %d): %w", pid, err)
		}
		if !waitExit(pid, stopTimeout) {
			return fmt.Errorf("meshd (pid %d) did not exit within %s", pid, stopTimeout)
		}
		stopped = true
	}
	removePIDFile()

	// mesh0 is a persistent TUN device: it outlives meshd, and so would its route.
	removed := false
	if exec.Command("ip", "link", "show", meshIface).Run() == nil {
		if err := execCmd("ip", "link", "del", meshIface); err != nil {
			return err
		}
		removed = true
	}

	switch {
	case stopped:
		fmt.Printf("meshd stopped, %s removed\n", meshIface)
	case removed:
		fmt.Printf("meshd was not running; removed the leftover %s\n", meshIface)
	default:
		fmt.Println("meshd was not running")
	}
	return nil
}

// checkNotRunning fails when a meshd is already carrying traffic, before run
// touches mesh0. Restarting the service over its own process is fine.
func checkNotRunning(asService bool) error {
	pid, ok := runningPID()
	if !ok || pid == os.Getpid() {
		return nil
	}
	if asService && serviceActive() {
		return nil
	}
	return fmt.Errorf("meshd is already running (pid %d) — `meshd stop` it first", pid)
}

func serviceActive() bool {
	return exec.Command("systemctl", "is-active", "--quiet", serviceName).Run() == nil
}

func writePIDFile() error {
	if pid, ok := runningPID(); ok && pid != os.Getpid() {
		return fmt.Errorf("meshd is already running (pid %d) — `meshd stop` it first", pid)
	}
	if err := os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", pidPath, err)
	}
	return nil
}

func removePIDFile() {
	if pid, ok := runningPID(); ok && pid != os.Getpid() {
		return
	}
	_ = os.Remove(pidPath)
}

// runningPID returns the PID in the PID file when that process is still alive.
func runningPID() (int, bool) {
	buf, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(buf)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, processAlive(pid)
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

func waitExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !processAlive(pid)
}
