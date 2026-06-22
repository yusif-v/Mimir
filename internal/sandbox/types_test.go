package sandbox

import "testing"

func TestSandboxStatusConstants(t *testing.T) {
	tests := []struct {
		status SandboxStatus
		want   string
	}{
		{SandboxCreated, "created"},
		{SandboxRunning, "running"},
		{SandboxStopped, "stopped"},
		{SandboxDestroyed, "destroyed"},
	}
	for _, tt := range tests {
		if string(tt.status) != tt.want {
			t.Errorf("got %q, want %q", tt.status, tt.want)
		}
	}
}

func TestCaptureEventTypeConstants(t *testing.T) {
	tests := []struct {
		typ  CaptureEventType
		want string
	}{
		{EventFileCreate, "file_create"},
		{EventFileModify, "file_modify"},
		{EventFileDelete, "file_delete"},
		{EventProcessStart, "process_start"},
		{EventProcessExit, "process_exit"},
		{EventNetworkAttempt, "network_attempt"},
	}
	for _, tt := range tests {
		if string(tt.typ) != tt.want {
			t.Errorf("got %q, want %q", tt.typ, tt.want)
		}
	}
}
