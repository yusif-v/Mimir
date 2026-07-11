package cases

import (
	"strings"
	"testing"
)

func newReportCase(t *testing.T) *Case {
	t.Helper()
	c := &Case{Name: "incident-7", Status: "open", Path: t.TempDir()}
	if err := c.Scaffold(); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := c.AppendEvidence(EvidenceRecord{Op: "add", Name: "evil.exe", SHA256: "abc123", Size: 4096, Tags: []string{"malware"}}); err != nil {
		t.Fatalf("append evidence: %v", err)
	}
	if err := c.AppendIOC(IOCRecord{Type: "ip", Value: "1.2.3.4", Source: "strings"}); err != nil {
		t.Fatalf("append ioc: %v", err)
	}
	if err := c.AppendEvent(TimelineEvent{Type: "evidence_added", Timestamp: "2026-07-11T10:00:00Z", Payload: map[string]any{"name": "evil.exe"}}); err != nil {
		t.Fatalf("append event: %v", err)
	}
	return c
}

func TestBuildReportContainsSections(t *testing.T) {
	c := newReportCase(t)
	md, err := BuildReport(c, "Summary text here.")
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	for _, want := range []string{"# Incident Report: incident-7", "## Evidence", "evil.exe", "## Indicators of Compromise", "1.2.3.4", "## Timeline", "## AI Executive Summary", "Summary text here."} {
		if !strings.Contains(md, want) {
			t.Errorf("report missing %q\n---\n%s", want, md)
		}
	}
}

func TestBuildReportNoAI(t *testing.T) {
	c := newReportCase(t)
	md, err := BuildReport(c, "")
	if err != nil {
		t.Fatalf("BuildReport: %v", err)
	}
	if strings.Contains(md, "AI Executive Summary") {
		t.Error("AI section should be absent when summary is empty")
	}
}

func TestBuildNarrativePhaseLabels(t *testing.T) {
	c := newReportCase(t)
	narr, err := c.Narrative()
	if err != nil {
		t.Fatalf("Narrative: %v", err)
	}
	if !strings.Contains(narr, "Acquisition & Analysis") {
		t.Errorf("expected a phase label, got:\n%s", narr)
	}
	if !strings.Contains(narr, "evidence_added") {
		t.Errorf("expected the timeline event type in narrative, got:\n%s", narr)
	}
}

func TestBuildNarrativeEmpty(t *testing.T) {
	c := &Case{Name: "empty", Path: t.TempDir()}
	if err := c.Scaffold(); err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	narr, err := c.Narrative()
	if err != nil {
		t.Fatalf("Narrative: %v", err)
	}
	if !strings.Contains(narr, "No timeline events") {
		t.Errorf("expected empty-marker, got: %s", narr)
	}
}
