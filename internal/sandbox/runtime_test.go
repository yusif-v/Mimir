package sandbox

import (
	"os/exec"
	"testing"
)

func TestDockerAvailable(t *testing.T) {
	// This test checks if docker is available — skip if not
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	// Just verify the check function works
	_ = dockerAvailable()
}

func TestRuntimeCreateContainer(t *testing.T) {
	if !dockerAvailable() {
		t.Skip("docker not available")
	}
	// Integration test — requires Docker
}
