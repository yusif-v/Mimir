package shell

import "encoding/json"

// capaHint is an ATT&CK-mapped capability extracted from capa's JSON output.
type capaHint struct{ typ, val string }

type capaAttack struct {
	Technique    string `json:"technique"`
	Subtechnique string `json:"subtechnique"`
	ID           string `json:"id"`
}

type capaRuleMeta struct {
	Attack []capaAttack `json:"attack"`
}

type capaRule struct {
	Meta capaRuleMeta `json:"meta"`
}

type capaOutput struct {
	Rules map[string]capaRule `json:"rules"`
}

// capaHints parses capa's `-j` JSON output into one hint per ATT&CK-mapped
// rule match. Rules with no ATT&CK mapping (an empty "attack" list — capa
// does not map every rule to a technique) are skipped. Malformed/empty input
// yields no hints rather than an error, matching the other *Hints parsers'
// best-effort style (regripperHints, volatilityHints).
func capaHints(out string) []capaHint {
	var parsed capaOutput
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		return nil
	}
	var hints []capaHint
	for _, rule := range parsed.Rules {
		for _, a := range rule.Meta.Attack {
			val := a.ID + " " + a.Technique
			if a.Subtechnique != "" {
				val += ": " + a.Subtechnique
			}
			hints = append(hints, capaHint{typ: "capability", val: val})
		}
	}
	return hints
}
