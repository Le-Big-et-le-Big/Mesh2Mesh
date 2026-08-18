package main

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const unitFile = `[Unit]
Description=Meshing VPN service

[Service]
Type=simple
Restart=always
ExecStart=/home/kilian/projets/Mesh2Mesh/Mesh2Mesh

[Install]
WantedBy=multi-user.target`

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	// Create a systemd unit file.
	path1 := filepath.Join("/etc/systemd/system", "mesh2mesh.service")
	err := os.WriteFile(path1, []byte(unitFile), 0644)
	check(err)

	// Reload systemd so it picks up the new unit file
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		log.Fatal(err)
	}

	// Start the service
	if err := exec.Command("systemctl", "start", "mesh2mesh.service").Run(); err != nil {
		log.Fatal(err)
	}
}
