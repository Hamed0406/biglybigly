package netmon

import (
	"testing"
)

func TestParseState(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"01", "ESTABLISHED"},
		{"06", "TIME_WAIT"},
		{"0A", "LISTEN"},
		{"0a", "LISTEN"},
		{"FF", "FF"},
		{"", ""},
	}
	for _, tt := range tests {
		got := parseState(tt.input)
		if got != tt.want {
			t.Errorf("parseState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFilterAndEnrich(t *testing.T) {
	flows := []Flow{
		{Proto: "tcp", RemoteIP: "1.2.3.4", RemotePort: 443},
		{Proto: "tcp", RemoteIP: "127.0.0.1", RemotePort: 80},    // loopback — filtered
		{Proto: "tcp", RemoteIP: "::1", RemotePort: 443},          // loopback — filtered
		{Proto: "tcp", RemoteIP: "0.0.0.0", RemotePort: 80},      // any — filtered
		{Proto: "tcp", RemoteIP: "10.0.0.1", RemotePort: 0},      // no port — filtered
		{Proto: "udp", RemoteIP: "8.8.8.8", RemotePort: 53},
	}

	result := filterAndEnrich(flows)
	if len(result) != 2 {
		t.Fatalf("expected 2 flows after filtering, got %d", len(result))
	}
	if result[0].RemoteIP != "1.2.3.4" {
		t.Errorf("expected first flow to be 1.2.3.4, got %s", result[0].RemoteIP)
	}
	if result[1].RemoteIP != "8.8.8.8" {
		t.Errorf("expected second flow to be 8.8.8.8, got %s", result[1].RemoteIP)
	}
	for _, f := range result {
		if f.SeenAt == 0 {
			t.Error("SeenAt should be set by filterAndEnrich")
		}
	}
}
