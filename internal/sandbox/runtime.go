package sandbox

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runtime manages Docker container operations for sandboxes.
type Runtime struct {
	dryRun bool // for testing without Docker
}

// NewRuntime creates a new Runtime instance.
func NewRuntime() *Runtime {
	return &Runtime{}
}

// dockerAvailable checks if Docker is installed and running.
func dockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

// CreateContainer creates a Docker container for the given sandbox.
func (r *Runtime) CreateContainer(s *Sandbox) error {
	if r.dryRun {
		s.ContainerID = "dry-run-" + s.ID
		s.Status = SandboxCreated
		return nil
	}
	if !dockerAvailable() {
		return fmt.Errorf("docker is not available — is Docker running?")
	}

	// Pull image if needed
	pullCmd := exec.Command("docker", "pull", s.Image)
	if output, err := pullCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker pull %s: %s", s.Image, string(output))
	}

	// Create container with no network, read-only root, limited resources
	args := []string{
		"run", "-d",
		"--name", s.ID,
		"--network", "none",
		"--read-only",
		"--memory", "512m",
		"--cpus", "1.0",
		"--tmpfs", "/tmp:rw,noexec,nosuid,size=100m",
		s.Image,
		"sleep", fmt.Sprintf("%d", s.Timeout),
	}

	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run: %s", string(output))
	}

	s.ContainerID = strings.TrimSpace(string(output))
	s.Status = SandboxCreated
	return nil
}

// CopyFile copies a file from the host into the container.
func (r *Runtime) CopyFile(containerID, srcPath, destPath string) error {
	if r.dryRun {
		return nil
	}
	cmd := exec.Command("docker", "cp", srcPath, containerID+":"+destPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker cp: %s", string(output))
	}
	return nil
}

// Exec runs a command inside the container and returns the output.
func (r *Runtime) Exec(containerID, command string) (string, error) {
	if r.dryRun {
		return "dry-run output", nil
	}
	cmd := exec.Command("docker", "exec", containerID, "sh", "-c", command)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// StartCapture starts background capture agents inside the container.
func (r *Runtime) StartCapture(containerID string) error {
	if r.dryRun {
		return nil
	}
	// Start background capture agents inside container
	// For now, run a simple monitoring script
	captureScript := `
#!/bin/bash
inotifywait -m -r -e create,modify,delete,move / 2>/dev/null | while read line; do
    echo "$(date -Iseconds) FILE: $line"
done &
ps aux > /tmp/process_snapshot_$(date +%s).txt
`
	cmd := exec.Command("docker", "exec", containerID, "sh", "-c", captureScript)
	return cmd.Start()
}

// RunTarget copies the target file into the container and executes it.
func (r *Runtime) RunTarget(containerID, targetFile string) (string, error) {
	if r.dryRun {
		return "dry-run execution", nil
	}
	// Copy file to /tmp/ in container and execute
	destPath := "/tmp/" + filepath.Base(targetFile)
	if err := r.CopyFile(containerID, targetFile, destPath); err != nil {
		return "", err
	}
	// Make executable and run
	cmd := exec.Command("docker", "exec", containerID, "sh", "-c",
		fmt.Sprintf("chmod +x %s && timeout %d %s", destPath, 60, destPath))
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// GetLogs retrieves the container's stdout/stderr logs.
func (r *Runtime) GetLogs(containerID string) (string, error) {
	if r.dryRun {
		return "dry-run logs", nil
	}
	cmd := exec.Command("docker", "logs", containerID)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// StopContainer stops the container gracefully with a 10-second timeout.
func (r *Runtime) StopContainer(containerID string) error {
	if r.dryRun {
		return nil
	}
	cmd := exec.Command("docker", "stop", "-t", "10", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker stop: %s", string(output))
	}
	return nil
}

// RemoveContainer forcefully removes the container.
func (r *Runtime) RemoveContainer(containerID string) error {
	if r.dryRun {
		return nil
	}
	cmd := exec.Command("docker", "rm", "-f", containerID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker rm: %s", string(output))
	}
	return nil
}

// Shell opens an interactive shell in the container.
func (r *Runtime) Shell(containerID string) error {
	if r.dryRun {
		return fmt.Errorf("dry-run: cannot open shell")
	}
	// This will be called from the shell command handler
	// It replaces the current process with docker exec -it
	cmd := exec.Command("docker", "exec", "-it", containerID, "/bin/bash")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
