package dnsfilter

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// QueryLog represents a single DNS query for batching to server
type QueryLog struct {
	Domain     string `json:"domain"`
	QType      string `json:"qtype"`
	ClientIP   string `json:"client_ip"`
	Blocked    bool   `json:"blocked"`
	UpstreamMs int64  `json:"upstream_ms"`
	Answer     string `json:"answer"`
	Timestamp  int64  `json:"timestamp"`
}

// Proxy is the DNS proxy server that runs on agents
type Proxy struct {
	listenAddr string
	upstream   []string
	blocklist  *BlocklistManager
	logger     *slog.Logger

	mu       sync.Mutex
	logBuf   []QueryLog
	server   *dns.Server
	server6  *dns.Server
}

// NewProxy creates a DNS proxy
func NewProxy(listenAddr string, upstream []string, blocklist *BlocklistManager, logger *slog.Logger) *Proxy {
	if len(upstream) == 0 {
		upstream = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}
	return &Proxy{
		listenAddr: listenAddr,
		upstream:   upstream,
		blocklist:  blocklist,
		logger:     logger,
	}
}

// Start runs the DNS proxy server (blocks until context is cancelled)
func (p *Proxy) Start(ctx context.Context) error {
	handler := dns.HandlerFunc(p.handleQuery)

	p.server = &dns.Server{
		Addr:    p.listenAddr,
		Net:     "udp",
		Handler: handler,
	}

	p.server6 = &dns.Server{
		Addr:    p.listenAddr,
		Net:     "tcp",
		Handler: handler,
	}

	errCh := make(chan error, 2)

	go func() {
		p.logger.Info("DNS proxy listening (UDP)", "addr", p.listenAddr)
		if err := p.server.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("UDP: %w", err)
		}
	}()

	go func() {
		p.logger.Info("DNS proxy listening (TCP)", "addr", p.listenAddr)
		if err := p.server6.ListenAndServe(); err != nil {
			errCh <- fmt.Errorf("TCP: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		p.server.Shutdown()
		p.server6.Shutdown()
		return nil
	case err := <-errCh:
		return err
	}
}

// FlushLogs returns and clears the query log buffer
func (p *Proxy) FlushLogs() []QueryLog {
	p.mu.Lock()
	defer p.mu.Unlock()

	logs := p.logBuf
	p.logBuf = nil
	return logs
}

func (p *Proxy) handleQuery(w dns.ResponseWriter, req *dns.Msg) {
	if len(req.Question) == 0 {
		dns.HandleFailed(w, req)
		return
	}

	q := req.Question[0]
	domain := normalizeDomain(q.Name)
	qtype := dns.TypeToString[q.Qtype]
	clientIP := extractClientIP(w.RemoteAddr())

	// Check blocklist
	if p.blocklist.IsBlocked(domain) {
		p.logger.Debug("DNS blocked", "domain", domain, "client", clientIP)

		resp := new(dns.Msg)
		resp.SetReply(req)
		resp.Authoritative = true

		// Return 0.0.0.0 for A queries, :: for AAAA
		switch q.Qtype {
		case dns.TypeA:
			resp.Answer = append(resp.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: q.Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
				A:   net.ParseIP("0.0.0.0"),
			})
		case dns.TypeAAAA:
			resp.Answer = append(resp.Answer, &dns.AAAA{
				Hdr:  dns.RR_Header{Name: q.Name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 300},
				AAAA: net.ParseIP("::"),
			})
		default:
			resp.Rcode = dns.RcodeNameError
		}

		w.WriteMsg(resp)

		p.addLog(QueryLog{
			Domain:    domain,
			QType:     qtype,
			ClientIP:  clientIP,
			Blocked:   true,
			Answer:    "0.0.0.0 (blocked)",
			Timestamp: time.Now().Unix(),
		})
		return
	}

	// Forward to upstream
	start := time.Now()
	var resp *dns.Msg
	var err error

	client := &dns.Client{
		Net:     "udp",
		Timeout: 5 * time.Second,
	}

	for _, upstream := range p.upstream {
		resp, _, err = client.Exchange(req, upstream)
		if err == nil {
			break
		}
	}

	upstreamMs := time.Since(start).Milliseconds()

	if err != nil || resp == nil {
		p.logger.Warn("DNS upstream failed", "domain", domain, "err", err)
		dns.HandleFailed(w, req)

		p.addLog(QueryLog{
			Domain:     domain,
			QType:      qtype,
			ClientIP:   clientIP,
			Blocked:    false,
			UpstreamMs: upstreamMs,
			Answer:     "SERVFAIL",
			Timestamp:  time.Now().Unix(),
		})
		return
	}

	w.WriteMsg(resp)

	// Extract first answer for logging
	answer := ""
	if len(resp.Answer) > 0 {
		answer = extractAnswer(resp.Answer[0])
	}

	p.addLog(QueryLog{
		Domain:     domain,
		QType:      qtype,
		ClientIP:   clientIP,
		Blocked:    false,
		UpstreamMs: upstreamMs,
		Answer:     answer,
		Timestamp:  time.Now().Unix(),
	})
}

func (p *Proxy) addLog(log QueryLog) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Cap buffer to prevent unbounded growth
	if len(p.logBuf) >= 10000 {
		// Drop oldest 20%
		p.logBuf = p.logBuf[2000:]
	}
	p.logBuf = append(p.logBuf, log)
}

func extractClientIP(addr net.Addr) string {
	if addr == nil {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String()
	}
	return host
}

func extractAnswer(rr dns.RR) string {
	switch v := rr.(type) {
	case *dns.A:
		return v.A.String()
	case *dns.AAAA:
		return v.AAAA.String()
	case *dns.CNAME:
		return strings.TrimSuffix(v.Target, ".")
	case *dns.MX:
		return strings.TrimSuffix(v.Mx, ".")
	default:
		return rr.String()
	}
}
