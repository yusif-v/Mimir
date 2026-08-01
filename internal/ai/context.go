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
	// Local marks a locally-run provider (e.g. Ollama). When false — the safe
	// default — case data is sanitized before it leaves the host: evidence
	// file names are masked to evidence_<n> so PII embedded in filenames is
	// not sent to a third-party model. Set true for local/self-hosted models.
	Local bool
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
				// Remote providers get masked filenames — evidence names are
				// user-controlled and can embed PII (usernames, share paths).
				name := e.Name
				if !opts.Local {
					name = fmt.Sprintf("evidence_%d", i+1)
				}
				parts = append(parts, fmt.Sprintf("  - %s (sha256:%s, size:%d%s)", name, shortHash(e.SHA256), e.Size, tags))
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

// shortHash returns the first 12 hex chars of an evidence SHA-256, or a
// placeholder if the hash is empty/too short (evidence added without one).
func shortHash(hash string) string {
	if len(hash) < 12 {
		if hash == "" {
			return "no-hash"
		}
		return hash
	}
	return hash[:12]
}

// BuildAnalysisContext creates a focused context for case analysis.
func BuildAnalysisContext(c *cases.Case) []Message {
	return BuildAnalysisContextLocal(c, false)
}

// BuildAnalysisContextLocal is BuildAnalysisContext with an explicit local
// flag for PII-safe context over remote providers.
func BuildAnalysisContextLocal(c *cases.Case, local bool) []Message {
	opts := DefaultContextOptions()
	opts.Local = local
	opts.MaxTimelineEvents = 100
	opts.MaxEvidenceItems = 50
	opts.MaxIOCs = 100
	return BuildContext(c, opts)
}

// BuildSuggestionContext creates a focused context for next-step suggestions.
func BuildSuggestionContext(c *cases.Case) []Message {
	return BuildSuggestionContextLocal(c, false)
}

// BuildSuggestionContextLocal is BuildSuggestionContext with an explicit local
// flag for PII-safe context over remote providers.
func BuildSuggestionContextLocal(c *cases.Case, local bool) []Message {
	opts := DefaultContextOptions()
	opts.Local = local
	opts.MaxTimelineEvents = 20
	opts.MaxEvidenceItems = 10
	opts.MaxIOCs = 30
	return BuildContext(c, opts)
}
