package main

import (
	"testing"
)

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
