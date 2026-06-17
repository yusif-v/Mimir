package tools

import "testing"

func TestResolveStatus(t *testing.T) {
	dockerTool := &Definition{Name: "vol", DockerImage: "dfir-volatility:latest"}
	dockerNoTag := &Definition{Name: "x", DockerImage: "img"}
	localTool := &Definition{Name: "ls", LocalCmd: "ls"}

	cases := []struct {
		name       string
		def        *Definition
		imageSet   map[string]bool
		dockerUp   bool
		localFound bool
		want       Status
	}{
		{"docker ready", dockerTool, map[string]bool{"dfir-volatility:latest": true}, true, false, StatusReady},
		{"docker not built", dockerTool, map[string]bool{}, true, false, StatusNotBuilt},
		{"docker daemon down", dockerTool, map[string]bool{}, false, false, StatusDockerOff},
		{"untagged image normalizes to latest", dockerNoTag, map[string]bool{"img:latest": true}, true, false, StatusReady},
		{"local found", localTool, nil, true, true, StatusReady},
		{"local missing", localTool, nil, true, false, StatusMissing},
	}
	for _, tc := range cases {
		if got := resolveStatus(tc.def, tc.imageSet, tc.dockerUp, tc.localFound); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestStatusString(t *testing.T) {
	if StatusNotBuilt.String() != "not built" {
		t.Errorf("got %q", StatusNotBuilt.String())
	}
}
