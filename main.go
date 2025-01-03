package main

// Based on https://github.com/katakonst/go-dns-proxy/releases

import (
	"log"
	"net"
	"time"

	"github.com/miekg/dns"
)

func main() {
	cfg, err := InitConfig()
	if err != nil {
		log.Fatalf("Failed to load configs: %s", err)
	}

	if len(cfg.Zones["default"].Prefix) != net.IPv6len || cfg.Zones["default"].Prefix.IsUnspecified() {
		log.Fatalf("Wrong prefix format: %s", cfg.Zones["default"].Prefix)
	}

	dnsProxy := DNSProxy{
		Cache:          New(cfg.Cache.ExpTime*time.Minute, cfg.Cache.PurgeTime*time.Minute),
		forwarders:     cfg.Forwarders,
		static:         cfg.Static,
		defaultForward: cfg.Default,
		ia:             cfg.IA,
		zones:          cfg.Zones,
	}

	logger := NewLogger(cfg.LogLevel)

	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		switch r.Opcode {
		case dns.OpcodeQuery:
			m, err := dnsProxy.getResponse(r)
			if err != nil {
				logger.Errorf("Failed lookup for %s with error: %s\n", r, err.Error())
			}
			w.WriteMsg(m)
		}
	})

	server := &dns.Server{Addr: cfg.Listen, Net: "udp"}
	logger.Infof("Starting at %s\n", cfg.Listen)
	err = server.ListenAndServe()
	if err != nil {
		logger.Errorf("Failed to start server: %s\n ", err.Error())
	}
}
