package netmon

import (
	"testing"
)

// Golden fixtures for /proc/net/tcp parsing

func TestParseHexAddr_IPv4(t *testing.T) {
	tests := []struct {
		input    string
		wantIP   string
		wantPort int
	}{
		{"0100007F:0035", "127.0.0.1", 53},
		{"00000000:1F90", "0.0.0.0", 8080},
		{"0101A8C0:01BB", "192.168.1.1", 443},
		{"", "", 0},
		{"invalid", "", 0},
	}
	for _, tt := range tests {
		ip, port := ParseHexAddr(tt.input)
		if ip != tt.wantIP || port != tt.wantPort {
			t.Errorf("ParseHexAddr(%q) = (%q, %d), want (%q, %d)", tt.input, ip, port, tt.wantIP, tt.wantPort)
		}
	}
}

func TestParseHexAddr_IPv6(t *testing.T) {
	// IPv6 loopback ::1 in /proc format: 00000000000000000000000001000000
	ip, port := ParseHexAddr("00000000000000000000000001000000:0050")
	if ip != "::1" || port != 80 {
		t.Errorf("IPv6 loopback: got (%q, %d), want (\"::1\", 80)", ip, port)
	}

	// All zeros
	ip2, port2 := ParseHexAddr("00000000000000000000000000000000:0000")
	if ip2 != "::" || port2 != 0 {
		t.Errorf("IPv6 zero: got (%q, %d), want (\"::\", 0)", ip2, port2)
	}
}

// Fixture: a realistic /proc/net/tcp file snippet
const procNetTCPFixture = `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0100007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 12345 1 0000000000000000 100 0 0 10 0
   1: 0101A8C0:C5E2 2E50FA8E:01BB 01 00000000:00000000 02:0000054C 00000000  1000        0 67890 1 0000000000000000 20 4 24 10 -1
   2: 0100007F:1F90 0100007F:E234 01 00000000:00000000 00:00000000 00000000  1000        0 11111 1 0000000000000000 20 4 30 10 -1`

func TestParseProcNetFixture(t *testing.T) {
	// Write fixture to temp file
	tmpFile := t.TempDir() + "/tcp"
	if err := writeFixture(tmpFile, procNetTCPFixture); err != nil {
		t.Fatal(err)
	}

	flows := parseProcNet(tmpFile, "tcp")
	if len(flows) != 3 {
		t.Fatalf("expected 3 flows, got %d", len(flows))
	}

	// First entry: loopback DNS listener
	if flows[0].LocalIP != "127.0.0.1" || flows[0].LocalPort != 53 {
		t.Errorf("flow 0: got %s:%d, want 127.0.0.1:53", flows[0].LocalIP, flows[0].LocalPort)
	}
	if flows[0].State != "LISTEN" {
		t.Errorf("flow 0 state: got %s, want LISTEN", flows[0].State)
	}

	// Second entry: outbound HTTPS connection (192.168.1.1:50658 -> 142.80.250.46:443)
	if flows[1].RemotePort != 443 {
		t.Errorf("flow 1 remote port: got %d, want 443", flows[1].RemotePort)
	}
	if flows[1].State != "ESTABLISHED" {
		t.Errorf("flow 1 state: got %s, want ESTABLISHED", flows[1].State)
	}

	// Third entry: local connection (loopback to loopback)
	if flows[2].LocalIP != "127.0.0.1" || flows[2].RemoteIP != "127.0.0.1" {
		t.Errorf("flow 2: expected loopback-to-loopback")
	}
}

func TestParseProcNet_MissingFile(t *testing.T) {
	flows := parseProcNet("/nonexistent/path", "tcp")
	if flows != nil {
		t.Errorf("expected nil for missing file, got %d flows", len(flows))
	}
}

func writeFixture(path, content string) error {
	return writeFile(path, []byte(content))
}
