package shell

import "testing"

const capaFixtureJSON = `{
  "rules": {
    "inject thread into remote process": {
      "meta": {
        "attack": [
          {"tactic": "Defense Evasion", "technique": "Process Injection", "subtechnique": "", "id": "T1055"}
        ]
      }
    },
    "encode data using base64": {
      "meta": {
        "attack": [
          {"tactic": "Defense Evasion", "technique": "Obfuscated Files or Information", "subtechnique": "Standard Encoding", "id": "T1027.010"}
        ]
      }
    },
    "read file on Windows": {
      "meta": {"attack": []}
    }
  }
}`

func TestCapaHints(t *testing.T) {
	hints := capaHints(capaFixtureJSON)
	if len(hints) != 2 {
		t.Fatalf("expected 2 capability hints (rule with no ATT&CK mapping skipped), got %d: %+v", len(hints), hints)
	}
	want := map[string]bool{
		"T1055 Process Injection":                                      false,
		"T1027.010 Obfuscated Files or Information: Standard Encoding": false,
	}
	for _, h := range hints {
		if h.typ != "capability" {
			t.Errorf("expected typ=capability, got %q", h.typ)
		}
		if _, ok := want[h.val]; !ok {
			t.Errorf("unexpected hint value: %q", h.val)
		}
		want[h.val] = true
	}
	for val, seen := range want {
		if !seen {
			t.Errorf("missing expected hint: %q", val)
		}
	}
}

func TestCapaHintsEmptyOrInvalid(t *testing.T) {
	if hints := capaHints(""); len(hints) != 0 {
		t.Errorf("empty input: expected no hints, got %+v", hints)
	}
	if hints := capaHints("not json"); len(hints) != 0 {
		t.Errorf("invalid json: expected no hints, got %+v", hints)
	}
}
