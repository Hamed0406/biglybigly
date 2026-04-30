package netmon

import (
	"testing"
)

func TestParseLsofFields(t *testing.T) {
	// Golden fixture: lsof -F pcPtTn output
	fixture := `p1234
ccurl
fcwd
Ptcp
TST=ESTABLISHED
n192.168.1.5:52341->142.250.80.46:443
p5678
cchrome
Ptcp
TST=ESTABLISHED
n10.0.0.5:54000->151.101.1.140:443
Pudp
n10.0.0.5:55000->8.8.8.8:53
p9999
cnode
Ptcp
TST=LISTEN
n*:3000
`

	flows := parseLsofFields(fixture)

	// Should get 3 flows (listening socket filtered out by no "->")
	if len(flows) != 3 {
		t.Fatalf("expected 3 flows, got %d", len(flows))
	}

	// First flow: curl -> google
	if flows[0].PID != 1234 || flows[0].Process != "curl" {
		t.Errorf("flow 0: pid=%d process=%s, want 1234/curl", flows[0].PID, flows[0].Process)
	}
	if flows[0].RemoteIP != "142.250.80.46" || flows[0].RemotePort != 443 {
		t.Errorf("flow 0 remote: %s:%d, want 142.250.80.46:443", flows[0].RemoteIP, flows[0].RemotePort)
	}
	if flows[0].Proto != "tcp" {
		t.Errorf("flow 0 proto: %s, want tcp", flows[0].Proto)
	}
	if flows[0].State != "ESTABLISHED" {
		t.Errorf("flow 0 state: %s, want ESTABLISHED", flows[0].State)
	}

	// Second flow: chrome -> reddit
	if flows[1].PID != 5678 || flows[1].Process != "chrome" {
		t.Errorf("flow 1: pid=%d process=%s, want 5678/chrome", flows[1].PID, flows[1].Process)
	}
	if flows[1].Proto != "tcp" {
		t.Errorf("flow 1 proto: %s, want tcp", flows[1].Proto)
	}

	// Third flow: chrome UDP -> DNS
	if flows[2].PID != 5678 || flows[2].Proto != "udp" {
		t.Errorf("flow 2: pid=%d proto=%s, want 5678/udp", flows[2].PID, flows[2].Proto)
	}
	if flows[2].RemoteIP != "8.8.8.8" || flows[2].RemotePort != 53 {
		t.Errorf("flow 2 remote: %s:%d, want 8.8.8.8:53", flows[2].RemoteIP, flows[2].RemotePort)
	}
}

func TestParseLsofFields_IPv6(t *testing.T) {
	fixture := `p100
cssh
Ptcp6
TST=ESTABLISHED
n[::1]:22->[2001:db8::1]:54321
`
	flows := parseLsofFields(fixture)
	if len(flows) != 1 {
		t.Fatalf("expected 1 flow, got %d", len(flows))
	}
	if flows[0].RemoteIP != "2001:db8::1" || flows[0].RemotePort != 54321 {
		t.Errorf("IPv6 flow: %s:%d", flows[0].RemoteIP, flows[0].RemotePort)
	}
}

func TestParseLsofFields_Empty(t *testing.T) {
	flows := parseLsofFields("")
	if len(flows) != 0 {
		t.Errorf("expected 0 flows from empty input, got %d", len(flows))
	}
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		input    string
		wantIP   string
		wantPort int
	}{
		{"192.168.1.5:443", "192.168.1.5", 443},
		{"[::1]:22", "::1", 22},
		{"[2001:db8::1]:54321", "2001:db8::1", 54321},
		{"*:3000", "*", 3000},
		{"noport", "noport", 0},
	}
	for _, tt := range tests {
		ip, port := splitHostPort(tt.input)
		if ip != tt.wantIP || port != tt.wantPort {
			t.Errorf("splitHostPort(%q) = (%q, %d), want (%q, %d)", tt.input, ip, port, tt.wantIP, tt.wantPort)
		}
	}
}
