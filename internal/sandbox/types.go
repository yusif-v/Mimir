// Package sandbox manages Docker-based isolated environments for file analysis.
package sandbox

import (
	"fmt"
	"time"
)

// SandboxStatus represents the lifecycle state of a sandbox.
type SandboxStatus string

const (
	SandboxCreated   SandboxStatus = "created"
	SandboxRunning   SandboxStatus = "running"
	SandboxStopped   SandboxStatus = "stopped"
	SandboxDestroyed SandboxStatus = "destroyed"
)

// CaptureEventType represents a type of event captured during sandbox execution.
type CaptureEventType string

const (
	EventFileCreate     CaptureEventType = "file_create"
	EventFileModify     CaptureEventType = "file_modify"
	EventFileDelete     CaptureEventType = "file_delete"
	EventFileRename     CaptureEventType = "file_rename"
	EventProcessStart   CaptureEventType = "process_start"
	EventProcessExit    CaptureEventType = "process_exit"
	EventNetworkAttempt CaptureEventType = "network_attempt"
)

// NetworkMode represents the network configuration of a sandbox.
type NetworkMode string

const (
	NetworkNone NetworkMode = "none"
)

// Sandbox represents a Docker-based isolated environment.
type Sandbox struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	CaseID      string        `json:"case_id"`
	OS          string        `json:"os"`
	Image       string        `json:"image"`
	ContainerID string        `json:"container_id,omitempty"`
	Status      SandboxStatus `json:"status"`
	CreatedAt   string        `json:"created_at"`
	DestroyedAt string        `json:"destroyed_at,omitempty"`
	Timeout     int           `json:"timeout"`
	Network     NetworkMode   `json:"network"`
	TargetFile  string        `json:"target_file,omitempty"`
	Files       []string      `json:"files"`
}

// CaptureEvent represents a single captured event during sandbox execution.
type CaptureEvent struct {
	Timestamp string           `json:"timestamp"`
	Type      CaptureEventType `json:"type"`
	Details   string           `json:"details"`
}

// AnalysisReport contains the full analysis results from a sandbox run.
type AnalysisReport struct {
	SandboxID       string           `json:"sandbox_id"`
	StartedAt       string           `json:"started_at"`
	FinishedAt      string           `json:"finished_at,omitempty"`
	Events          []CaptureEvent   `json:"events"`
	FileChanges     []FileChange     `json:"file_changes"`
	Processes       []ProcessInfo    `json:"processes"`
	NetworkAttempts []NetworkAttempt `json:"network_attempts"`
}

// FileChange represents a filesystem change in the sandbox.
type FileChange struct {
	Path string `json:"path"`
	Type string `json:"type"` // create, modify, delete, rename
}

// ProcessInfo represents a process that ran in the sandbox.
type ProcessInfo struct {
	PID     int    `json:"pid"`
	Command string `json:"command"`
	Started string `json:"started"`
	Exited  string `json:"exited,omitempty"`
}

// NetworkAttempt represents a network connection attempt from the sandbox.
type NetworkAttempt struct {
	Protocol string `json:"protocol"`
	DestAddr string `json:"dest_addr"`
	DestPort int    `json:"dest_port"`
	Time     string `json:"time"`
}

// NewSandbox creates a new sandbox with the given parameters.
// If timeout is <= 0, it defaults to 300 seconds (5 minutes).
func NewSandbox(name, caseID, os, image, targetFile string, timeout int) *Sandbox {
	if timeout <= 0 {
		timeout = 300 // default 5 minutes
	}
	return &Sandbox{
		ID:         generateID(),
		Name:       name,
		CaseID:     caseID,
		OS:         os,
		Image:      image,
		Status:     SandboxCreated,
		CreatedAt:  time.Now().Format(time.RFC3339),
		Timeout:    timeout,
		Network:    NetworkNone,
		TargetFile: targetFile,
		Files:      []string{},
	}
}

// generateID creates a unique sandbox ID based on the current timestamp.
func generateID() string {
	return fmt.Sprintf("sbx-%d", time.Now().UnixNano())
}
