package main

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestProcessTypeA(t *testing.T) {
	handler := initDnsHandler()
	// Start the mock DNS server
	server, serverAddr := startMockDNSServer(t, handler)
	defer server.Shutdown()
	proxy := &DNSProxy{
		zones: map[string]ZoneConfig{
			"zone1": {ReturnPublicIPv4: true},
			"zone2": {ReturnPublicIPv4: false},
		},
	}

	tests := []struct {
		name         string
		zoneID       string
		query        string
		expectedData []string
	}{
		{"Success with ReturnPublicIPv4", "zone1", "v4only.com.", []string{"192.168.1.1"}},
		{"Success without ReturnPublicIPv4", "zone2", "v4only.com.", []string{}},
		{"Multiple records with ReturnPublicIPv4", "zone1", "v4multi.com.", []string{"192.168.1.4", "192.168.1.5"}},
		{"Multiple records without ReturnPublicIPv4", "zone2", "v4multi.com.", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := &dns.Question{Name: tt.query, Qtype: dns.TypeA}
			requestMsg := &dns.Msg{Question: []dns.Question{*q}}

			// Call processTypeA
			resp, err := proxy.processTypeA(serverAddr, lookup, q, requestMsg, tt.zoneID)
			if err != nil {
				t.Errorf("processTypeA() error = %v", err)
			}
			// Check length
			if len(resp.Answer) != len(tt.expectedData) {
				t.Errorf("processTypeA() response length = %v, want %v", len(resp.Answer), len(tt.expectedData))
			}

			// Validate all expected records
			foundData := make(map[string]bool)
			for _, rr := range resp.Answer {
				switch r := rr.(type) {
				case *dns.A:
					foundData[r.A.String()] = true
				case *dns.AAAA:
					foundData[r.AAAA.String()] = true
				case *dns.CNAME:
					foundData[r.Target] = true
				default:
					t.Errorf("Unexpected record type %T for %s", rr, tt.query)
				}
			}
			// Check if all expected data are in the response
			for _, data := range tt.expectedData {
				if !foundData[data] {
					t.Errorf("processTypeA() response does not contain expected data = %v", data)
				}
			}
			// Ensure no unexpected data is present
			for data := range foundData {
				found := false
				for _, expected := range tt.expectedData {
					if data == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Unexpected record %s found in response for %s", data, tt.query)
				}
			}
		})
	}
}

func TestGetZoneID(t *testing.T) {
	// Mock data for zones
	zones := map[string]ZoneConfig{
		"zone1":   {Domains: []string{"example.com", "test.com"}},
		"default": {Domains: []string{"."}},
	}

	dnsProxy := DNSProxy{zones: zones}

	tests := []struct {
		domain       string
		expectedZone string
	}{
		{"example.com.", "zone1"},                // Match example.com
		{"api.test.com.", "zone1"},               // Match test.com
		{"Example.com.", "zone1"},                // Case-insensitive match
		{"blog.subdomain.example.com.", "zone1"}, // Match 4th level subdomain
		{"supertest.com.", "default"},            // Tricky match to zone2
		{"test.com.example.org.", "default"},     // Match subdomain.example.org
		{"no-match-domain.com.", "default"},      // Default to .
		{"", "default"},                          // Empty domain
	}

	for _, tt := range tests {
		t.Run(tt.domain, func(t *testing.T) {
			result := dnsProxy.getZoneID(tt.domain)
			if result != tt.expectedZone {
				t.Errorf("getZoneID(%q) = %q; want %q", tt.domain, result, tt.expectedZone)
			}
		})
	}
}

// Mock DNS server setup
func startMockDNSServer(t *testing.T, handler dns.HandlerFunc) (*dns.Server, string) {
	udpAddr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0") // Use an available port
	if err != nil {
		t.Fatalf("Failed to resolve address: %v", err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		t.Fatalf("Failed to listen on UDP: %v", err)
	}

	server := &dns.Server{PacketConn: conn, Handler: handler}
	errChan := make(chan error)
	go func() {
		err := server.ActivateAndServe()
		if err != nil {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		t.Fatalf("Failed to start mock DNS server: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatalf("Failed to start mock DNS server: timeout")
	}

	t.Cleanup(func() {
		server.Shutdown()
	})

	return server, conn.LocalAddr().String()
}

// Init dns handler
func initDnsHandler() dns.HandlerFunc {
	// Mock DNS response
	handler := func(w dns.ResponseWriter, r *dns.Msg) {
		msg := new(dns.Msg)
		msg.SetReply(r)

		if len(r.Question) > 0 {
			switch r.Question[0].Name {
			case "v4only.com.":
				// Respond with an A record
				rr, _ := dns.NewRR("v4only.com. 3600 IN A 192.168.1.1")
				msg.Answer = append(msg.Answer, rr)
			case "alias.com.":
				// Respond with a CNAME record
				rr, _ := dns.NewRR("alias.com. 3600 IN CNAME v4only.com.")
				msg.Answer = append(msg.Answer, rr)
			case "longv4only.com.":
				// Respond with an A record
				rr, _ := dns.NewRR("longv4only.com. 3600 IN A 192.168.1.2")
				msg.Answer = append(msg.Answer, rr)
			case "v6only.com.":
				// Respond with an AAAA record
				rr, _ := dns.NewRR("v6only.com. 3600 IN AAAA 2001:db8::1")
				msg.Answer = append(msg.Answer, rr)
			case "v4multi.com.":
				// Respond with 2 A records
				rr1, _ := dns.NewRR("v4multi.com. 3600 IN A 192.168.1.4")
				msg.Answer = append(msg.Answer, rr1)
				rr2, _ := dns.NewRR("v4multi.com. 3600 IN A 192.168.1.5")
				msg.Answer = append(msg.Answer, rr2)
			case "v4v6both.com.":
				// Respond with an A and AAAA records
				rr1, _ := dns.NewRR("v4v6both.com. 3600 IN A 192.168.1.3")
				msg.Answer = append(msg.Answer, rr1)
				rr2, _ := dns.NewRR("v4v6both.com. 3600 IN AAAA 2001:db8::3")
				msg.Answer = append(msg.Answer, rr2)
			default:
				// Return an empty response for unknown queries
				msg.Rcode = dns.RcodeNameError
			}
		}

		w.WriteMsg(msg)
	}
	return handler
}

func TestLookup(t *testing.T) {
	handler := initDnsHandler()
	// Start the mock DNS server
	server, serverAddr := startMockDNSServer(t, handler)
	defer server.Shutdown()

	tests := []struct {
		name         string
		query        string
		expectedType string
		expectedData []string
	}{
		{"Example A Record", "v4only.com.", "A", []string{"192.168.1.1"}},
		{"Test CNAME Record", "alias.com.", "CNAME", []string{"v4only.com."}},
		{"Alias A Record", "longv4only.com.", "A", []string{"192.168.1.2"}},
		{"Example AAAA Record", "v6only.com.", "AAAA", []string{"2001:db8::1"}},
		{"Unknown Query", "unknown.com.", "", []string{}},
		{"Example A Record", "v4multi.com.", "A", []string{"192.168.1.4", "192.168.1.5"}},
		{"Example AAAA Record", "v4v6both.com.", "MIXED", []string{"192.168.1.3", "2001:db8::3"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a DNS query message
			query := new(dns.Msg)
			switch tt.expectedType {
			case "A":
				query.SetQuestion(tt.query, dns.TypeA)
			case "AAAA":
				query.SetQuestion(tt.query, dns.TypeAAAA)
			case "MIXED":
				query.SetQuestion(tt.query, dns.TypeANY)
			case "CNAME":
				query.SetQuestion(tt.query, dns.TypeCNAME)
			case "":
				query.SetQuestion(tt.query, dns.TypeA)
			}

			// Call the function under test
			resp, err := lookup(serverAddr, query)
			if tt.expectedType == "" {
				// Expecting an NXDOMAIN response for unknown queries
				if err != nil {
					t.Fatalf("Unexpected error for %s: %v", tt.query, err)
				}
				if resp == nil || resp.Rcode != dns.RcodeNameError {
					t.Fatalf("Expected NXDOMAIN for %s, but got: %v", tt.query, resp)
				}
				return
			}
			if err != nil {
				t.Fatalf("lookup() failed for %s: %v", tt.query, err)
			}
			if len(resp.Answer) == 0 {
				t.Fatalf("Expected answer for %s, but got none", tt.query)
			}

			// Validate all expected records
			foundData := make(map[string]bool)
			for _, rr := range resp.Answer {
				switch r := rr.(type) {
				case *dns.A:
					foundData[r.A.String()] = true
				case *dns.AAAA:
					foundData[r.AAAA.String()] = true
				case *dns.CNAME:
					foundData[r.Target] = true
				default:
					t.Errorf("Unexpected record type %T for %s", rr, tt.query)
				}
			}

			// Check if all expected data are in the response
			for _, expected := range tt.expectedData {
				if !foundData[expected] {
					t.Errorf("Expected record %s not found in response for %s", expected, tt.query)
				}
			}

			// Ensure no unexpected data is present
			for data := range foundData {
				found := false
				for _, expected := range tt.expectedData {
					if data == expected {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Unexpected record %s found in response for %s", data, tt.query)
				}
			}
		})
	}
}
