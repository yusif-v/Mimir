package ai

import (
	"fmt"
	"strings"

	"github.com/yusif-v/mimir/internal/cases"
)

// ContextOptions controls what case data is included in the LLM context.
type ContextOptions struct {
	// IncludeTimeline includes recent timeline events.
	IncludeTimeline bool
	// IncludeEvidence includes the evidence list.
	IncludeEvidence bool
	// IncludeIOCs includes tracked IOCs.
	IncludeIOCs bool
	// MaxTimelineEvents limits the number of timeline events (0 = all).
	MaxTimelineEvents int
	// MaxEvidenceItems limits the number of evidence items (0 = all).
	MaxEvidenceItems int
	// MaxIOCs limits the number of IOCs (0 = all).
	MaxIOCs int
}

// DefaultContextOptions returns options that include everything with reasonable limits.
func DefaultContextOptions() ContextOptions {
	return ContextOptions{
		IncludeTimeline:   true,
		IncludeEvidence:   true,
		IncludeIOCs:       true,
		MaxTimelineEvents: 50,
		MaxEvidenceItems:  20,
		MaxIOCs:           50,
	}
}

// SystemPromptPrefix is prepended to all AI queries.
const SystemPromptPrefix = `You are a DFIR (Digital Forensics and Incident Response) analyst assistant.
You have access to the investigation case data including evidence, IOCs, and timeline events.
Provide concise, technical analysis. Flag anything suspicious. Suggest next steps.
If you are unsure, say so. Do not fabricate findings.

`

// BuildContext converts case data into LLM messages.
func BuildContext(c *cases.Case, opts ContextOptions) []Message {
	if c == nil {
		return nil
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("Case: %s (status: %s)", c.Name, c.Status))

	if opts.IncludeEvidence {
		ev := c.Evidence()
		if len(ev) > 0 {
			parts = append(parts, fmt.Sprintf("\nEvidence (%d items):", len(ev)))
			limit := len(ev)
			if opts.MaxEvidenceItems > 0 && limit > opts.MaxEvidenceItems {
				limit = opts.MaxEvidenceItems
			}
			for i := 0; i < limit; i++ {
				e := ev[i]
				tags := ""
				if len(e.Tags) > 0 {
					tags = fmt.Sprintf(" [%s]", strings.Join(e.Tags, ","))
				}
				parts = append(parts, fmt.Sprintf("  - %s (sha256:%s, size:%d%s)", e.Name, e.SHA256[:12], e.Size, tags))
			}
		}
	}

	if opts.IncludeIOCs {
		iocs := c.IOCs()
		if len(iocs) > 0 {
			parts = append(parts, fmt.Sprintf("\nIOCs (%d tracked):", len(iocs)))
			limit := len(iocs)
			if opts.MaxIOCs > 0 && limit > opts.MaxIOCs {
				limit = opts.MaxIOCs
			}
			for i := 0; i < limit; i++ {
				ic := iocs[i]
				parts = append(parts, fmt.Sprintf("  - %s: %s (source:%s)", ic.Type, ic.Value, ic.Source))
			}
		}
	}

	if opts.IncludeTimeline {
		events := c.Timeline()
		if len(events) > 0 {
			parts = append(parts, fmt.Sprintf("\nTimeline (%d events):", len(events)))
			limit := len(events)
			if opts.MaxTimelineEvents > 0 && limit > opts.MaxTimelineEvents {
				limit = opts.MaxTimelineEvents
			}
			// Show most recent first.
			start := len(events) - limit
			if start < 0 {
				start = 0
			}
			for i := start; i < len(events); i++ {
				ev := events[i]
				payload := ""
				if len(ev.Payload) > 0 {
					payload = fmt.Sprintf(" — %v", ev.Payload)
				}
				parts = append(parts, fmt.Sprintf("  [%s] %s%s", ev.Timestamp, ev.Type, payload))
			}
		}
	}

	return []Message{
		{Role: RoleSystem, Content: SystemPromptPrefix + strings.Join(parts, "\n")},
	}
}

// BuildAnalysisContext creates a focused context for case analysis.
func BuildAnalysisContext(c *cases.Case) []Message {
	opts := DefaultContextOptions()
	opts.MaxTimelineEvents = 100
	opts.MaxEvidenceItems = 50
	opts.MaxIOCs = 100
	return BuildContext(c, opts)
}

// BuildSuggestionContext creates a focused context for next-step suggestions.
func BuildSuggestionContext(c *cases.Case) []Message {
	opts := DefaultContextOptions()
	opts.MaxTimelineEvents = 20
	opts.MaxEvidenceItems = 10
	opts.MaxIOCs = 30
	return BuildContext(c, opts)
}
