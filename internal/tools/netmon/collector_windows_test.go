package netmon

import (
	"testing"
)

func TestParsePSConnections(t *testing.T) {
	// Golden fixture: PowerShell Get-NetTCPConnection JSON output
	fixture := `[
		{
			"LocalAddress": "192.168.1.5",
			"LocalPort": 52341,
			"RemoteAddress": "142.250.80.46",
			"RemotePort": 443,
			"State": "Established",
			"OwningProcess": 1234
		},
		{
			"LocalAddress": "0.0.0.0",
			"LocalPort": 8082,
			"RemoteAddress": "0.0.0.0",
			"RemotePort": 0,
			"State": "Listen",
			"OwningProcess": 5678
		}
	]`

	flows := parsePSConnections([]byte(fixture), "tcp")
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(flows))
	}

	if flows[0].RemoteIP != "142.250.80.46" || flows[0].RemotePort != 443 {
		t.Errorf("flow 0: %s:%d, want 142.250.80.46:443", flows[0].RemoteIP, flows[0].RemotePort)
	}
	if flows[0].PID != 1234 {
		t.Errorf("flow 0 PID: %d, want 1234", flows[0].PID)
	}
	if flows[0].State != "ESTABLISHED" {
		t.Errorf("flow 0 state: %s, want ESTABLISHED", flows[0].State)
	}
	if flows[1].State != "LISTEN" {
		t.Errorf("flow 1 state: %s, want LISTEN", flows[1].State)
	}
}

func TestParsePSConnections_SingleObject(t *testing.T) {
	// PowerShell returns a single object (not array) if only one result
	fixture := `{
		"LocalAddress": "10.0.0.1",
		"LocalPort": 443,
		"RemoteAddress": "8.8.8.8",
		"RemotePort": 53,
		"State": "Established",
		"OwningProcess": 100
	}`

	flows := parsePSConnections([]byte(fixture), "udp")
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].Proto != "udp" {
		t.Errorf("proto: %s, want udp", flows[0].Proto)
	}
}

func TestParsePSConnections_Empty(t *testing.T) {
	flows := parsePSConnections([]byte(""), "tcp")
	if flows != nil {
		t.Errorf("expected nil from empty input, got %d flows", len(flows))
	}
}

func TestMapWindowsState(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Established", "ESTABLISHED"},
		{"Listen", "LISTEN"},
		{"TimeWait", "TIME_WAIT"},
		{"CloseWait", "CLOSE_WAIT"},
		{"SynSent", "SYN_SENT"},
		{"Bound", "BOUND"},
		{"Unknown", "Unknown"},
	}
	for _, tt := range tests {
		got := mapWindowsState(tt.input)
		if got != tt.want {
			t.Errorf("mapWindowsState(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseNetstat(t *testing.T) {
	fixture := `
Active Connections

  Proto  Local Address          Foreign Address        State           PID
  TCP    192.168.1.50:54321     142.250.74.46:443      ESTABLISHED     1234
  TCP    192.168.1.50:54322     20.205.243.166:443     ESTABLISHED     5678
  TCP    0.0.0.0:8082           0.0.0.0:0              LISTENING       9999
  TCP    127.0.0.1:49672        127.0.0.1:49673        ESTABLISHED     100
  TCP    [::1]:49670            [::1]:49671            ESTABLISHED     200
`

	flows := parseNetstat(fixture)
	if len(flows) != 5 {
		t.Fatalf("expected 5 flows, got %d", len(flows))
	}

	// Check first established connection
	if flows[0].RemoteIP != "142.250.74.46" || flows[0].RemotePort != 443 {
		t.Errorf("flow 0: %s:%d, want 142.250.74.46:443", flows[0].RemoteIP, flows[0].RemotePort)
	}
	if flows[0].LocalIP != "192.168.1.50" || flows[0].LocalPort != 54321 {
		t.Errorf("flow 0 local: %s:%d, want 192.168.1.50:54321", flows[0].LocalIP, flows[0].LocalPort)
	}
	if flows[0].PID != 1234 {
		t.Errorf("flow 0 PID: %d, want 1234", flows[0].PID)
	}
	if flows[0].Proto != "tcp" {
		t.Errorf("flow 0 proto: %s, want tcp", flows[0].Proto)
	}

	// Check IPv6 parsing
	if flows[4].LocalIP != "::1" || flows[4].LocalPort != 49670 {
		t.Errorf("flow 4 local: %s:%d, want [::1]:49670", flows[4].LocalIP, flows[4].LocalPort)
	}
}

func TestSplitNetstatAddr(t *testing.T) {
	tests := []struct {
		addr     string
		wantIP   string
		wantPort int
	}{
		{"192.168.1.50:443", "192.168.1.50", 443},
		{"0.0.0.0:8082", "0.0.0.0", 8082},
		{"[::1]:49670", "::1", 49670},
		{"[::]:445", "::", 445},
	}
	for _, tt := range tests {
		ip, port := splitNetstatAddr(tt.addr)
		if ip != tt.wantIP || port != tt.wantPort {
			t.Errorf("splitNetstatAddr(%q) = (%q, %d), want (%q, %d)", tt.addr, ip, port, tt.wantIP, tt.wantPort)
		}
	}
}
