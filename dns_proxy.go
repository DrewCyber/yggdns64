package main

import (
	"net"
	"strconv"
	"strings"

	"github.com/miekg/dns"

	//    "github.com/gdexlab/go-render/render"
	"fmt"
)

var yggnet *net.IPNet

type DNSProxy struct {
	Cache          *Cache
	static         map[string]string
	forwarders     map[string]string
	defaultForward string
	ia             InvalidAddress
	zones          map[string]ZoneConfig
}

func (proxy *DNSProxy) getResponse(requestMsg *dns.Msg) (*dns.Msg, error) {
	responseMsg := new(dns.Msg)
	var answer *dns.Msg
	var err error

	if len(requestMsg.Question) > 0 {
		question := requestMsg.Question[0]

		dnsServer := proxy.getForwarder(question.Name)
		zoneID := proxy.getZoneID(question.Name)

		// If zoneID is empty, return NXDOMAIN
		if zoneID == "" {
			responseMsg.SetRcode(requestMsg, dns.RcodeNameError)
			return responseMsg, nil
		}

		switch question.Qtype {
		case dns.TypeA:
			answer, err = proxy.processTypeA(dnsServer, lookup, &question, requestMsg, zoneID)
		case dns.TypeAAAA:
			answer, err = proxy.processTypeAAAA(dnsServer, &question, requestMsg, zoneID)
		case dns.TypePTR:
			answer, err = proxy.processTypePTR(dnsServer, &question, requestMsg, zoneID)
		case dns.TypeANY:
			answer, err = proxy.processTypeANY(dnsServer, &question, requestMsg, zoneID)
		default:
			answer, err = proxy.processOtherTypes(dnsServer, &question, requestMsg)
		}
	}

	if err != nil {
		return responseMsg, err
	}

	//    answer.MsgHdr.RecursionDesired = true
	answer.MsgHdr.RecursionAvailable = true
	return answer, err
}

func (proxy *DNSProxy) processOtherTypes(dnsServer string, q *dns.Question, requestMsg *dns.Msg) (*dns.Msg, error) {
	queryMsg := new(dns.Msg)
	requestMsg.CopyTo(queryMsg)
	queryMsg.Question = []dns.Question{*q}

	msg, err := lookup(dnsServer, queryMsg)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

// Query ANY
func (proxy *DNSProxy) processTypeANY(dnsServer string, q *dns.Question, requestMsg *dns.Msg, zoneID string) (*dns.Msg, error) {
	queryMsg := new(dns.Msg)
	requestMsg.CopyTo(queryMsg)
	queryMsg.Question = []dns.Question{*q}

	msg, err := lookup(dnsServer, queryMsg)
	if err != nil {
		return nil, err
	}

	// Recompile reply
	msg.Answer = proxy.processAnswerArray(msg.Answer, zoneID)
	msg.Extra = proxy.processAnswerArray(msg.Extra, zoneID)

	return msg, nil
}

// process answer array
func (proxy *DNSProxy) processAnswerArray(q []dns.RR, zoneID string) (answer []dns.RR) {
	answer = make([]dns.RR, 0)
	for _, orr := range q {
		switch rr := orr.(type) {
		case *dns.AAAA:
			if rr.AAAA.IsUnspecified() {
				switch proxy.ia {
				case DiscardInvalidAddress: // drop
					continue
				case IgnoreInvalidAddress: // also drop
					continue
				case ProcessInvalidAddress: // return "as-is"
					answer = append(answer, rr)
				}
			} else {
				// if answer contains ygg address - return it
				if yggnet.Contains(rr.AAAA) {
					answer = append(answer, rr)
				}
			}
		case *dns.A:
			if rr.A.IsUnspecified() {
				switch proxy.ia {
				case DiscardInvalidAddress: // drop
					continue
				case IgnoreInvalidAddress: // return "as-is"
				case ProcessInvalidAddress: // return "[::]"
					nrr, _ := dns.NewRR(rr.Hdr.Name + " IN AAAA ::")
					answer = append(answer, nrr)
					if proxy.zones[zoneID].ReturnPublicIPv4 {
						answer = append(answer, rr)
					}
					continue
				}
			}
			// return fake ip
			if proxy.zones[zoneID].Prefix != nil {
				nrr, _ := dns.NewRR(rr.Hdr.Name + " IN AAAA " + proxy.MakeFakeIP(rr.A, zoneID))
				answer = append(answer, nrr)
			}
			// return public ipv4
			if proxy.zones[zoneID].ReturnPublicIPv4 {
				answer = append(answer, rr)
			}
		default:
			answer = append(answer, rr)
		}
	}
	return
}

// Query PTR
func (proxy *DNSProxy) processTypePTR(dnsServer string, q *dns.Question, requestMsg *dns.Msg, zoneID string) (*dns.Msg, error) {
	queryMsg := new(dns.Msg)
	requestMsg.CopyTo(queryMsg)
	//    queryMsg.Question = []dns.Question{*q}

	ip, err := proxy.ReversePTR(q.Name, zoneID)
	if err != nil {
		queryMsg.MsgHdr.Rcode = dns.RcodeNameError
		queryMsg.MsgHdr.Opcode = dns.OpcodeNotify
		return queryMsg, nil
	}
	origQuestion := requestMsg.Question
	q.Name, _ = dns.ReverseAddr(ip.String())
	queryMsg.Question = []dns.Question{*q}

	msg, err := lookup(dnsServer, queryMsg)
	if err != nil {
		return nil, err
	}
	msg.Question = origQuestion
	answer := make([]dns.RR, 0)
	for _, orr := range msg.Answer {
		a, okA := orr.(*dns.PTR)
		if okA {
			rr, _ := dns.NewRR(origQuestion[0].Name + " IN PTR " + a.Ptr)
			answer = append(answer, rr)
		}
	}
	msg.Answer = answer
	msg.Question[0].Qtype = dns.TypePTR
	//fmt.Printf("\nPTR %s\n",render.Render(msg))
	return msg, nil
}

// Query A record.
func (proxy *DNSProxy) processTypeA(dnsServer string, lookup LookupFunc, q *dns.Question, requestMsg *dns.Msg, zoneID string) (*dns.Msg, error) {
	queryMsg := new(dns.Msg)
	requestMsg.CopyTo(queryMsg)
	queryMsg.Question = []dns.Question{*q}
	msg, err := lookup(dnsServer, queryMsg)
	if err != nil {
		queryMsg.MsgHdr.Rcode = dns.RcodeServerFailure
		queryMsg.MsgHdr.Opcode = dns.OpcodeNotify
		return queryMsg, err
	}
	if !proxy.zones[zoneID].ReturnPublicIPv4 {
		// Emulate "no record" for existings A
		msg.Answer = make([]dns.RR, 0)
	}
	return msg, nil
}

func (proxy *DNSProxy) processTypeAAAA(dnsServer string, q *dns.Question, requestMsg *dns.Msg, zoneID string) (msg *dns.Msg, err error) {
	msg = new(dns.Msg)
	cacheAnswer, found := proxy.Cache.Get(q.Name)

	// Have cache record?

	if !found {

		// No cache.
		// Have static address?

		ip := proxy.getStatic(q.Name)
		if ip != "" {
			requestMsg.CopyTo(msg)
			answer := make([]dns.RR, 0)
			if proxy.zones[zoneID].Prefix != nil {
				rr, _ := dns.NewRR(q.Name + " IN AAAA " + proxy.MakeFakeIP(net.ParseIP(ip), zoneID))
				answer = append(answer, rr)
			}
			msg.Answer = answer
			msg.Question[0].Qtype = dns.TypeAAAA
			msg.MsgHdr.Response = true
			proxy.Cache.Set(q.Name, answer, 0)
			return msg, nil
		}

		// No static.
		// Query AAAA address, may be it's already ygg?

		queryMsg := new(dns.Msg)
		requestMsg.CopyTo(queryMsg)
		queryMsg.Question = []dns.Question{*q}

		msg, err = lookup(dnsServer, queryMsg)
		if err != nil {
			return nil, err
		}

		answer := make([]dns.RR, 0)

		for _, orr := range msg.Answer {
			a, okA := orr.(*dns.AAAA)
			if okA {
				if yggnet.Contains(a.AAAA) {
					answer = append(answer, orr)
				}
			}
		}

		if len(answer) != 0 {
			msg.Answer = answer
			msg.MsgHdr.Response = true
			proxy.Cache.Set(q.Name, answer, 0)
			return msg, nil
		}

		// No. Ok, query A address and translate to ygg.

		q.Qtype = dns.TypeA
		queryMsg = new(dns.Msg)
		requestMsg.CopyTo(queryMsg)
		queryMsg.Question = []dns.Question{*q}

		msg, err = lookup(dnsServer, queryMsg)
		if err != nil {
			return nil, err
		}

		// Build fake answer

		answer = make([]dns.RR, 0)
		for _, orr := range msg.Answer {
			a, okA := orr.(*dns.A)
			if okA {
				if a.A.IsUnspecified() {
					switch proxy.ia {
					case DiscardInvalidAddress: // drop
						continue
					case IgnoreInvalidAddress: // return "as-is"
					case ProcessInvalidAddress: // return "[::]"
						nrr, _ := dns.NewRR(q.Name + " IN AAAA ::")
						answer = append(answer, nrr)
						continue
					}
				}
				if proxy.zones[zoneID].Prefix != nil {
					rr, _ := dns.NewRR(q.Name + " IN AAAA " + proxy.MakeFakeIP(a.A, zoneID))
					answer = append(answer, rr)
				}
			}
		}
		msg.Answer = answer
		msg.Question[0].Qtype = dns.TypeAAAA

		if len(answer) > 0 {
			proxy.Cache.Set(q.Name, answer, 0)
		}
		return msg, nil
	} else {

		// We have cache record

		requestMsg.CopyTo(msg)
		msg.Answer = cacheAnswer.([]dns.RR)
		msg.Question[0].Qtype = dns.TypeAAAA
		msg.MsgHdr.Response = true
		return msg, nil
	}
}

func (dnsProxy *DNSProxy) getForwarder(domain string) string {
	for k, v := range dnsProxy.forwarders {
		if strings.HasSuffix(strings.ToLower(domain), strings.ToLower(k+".")) {
			return v
		}
	}
	return dnsProxy.defaultForward
}

func (dnsProxy *DNSProxy) getZoneID(domain string) string {
	//  Find zone id for the requested domain
	for k, v := range dnsProxy.zones {
		for _, ZoneDomain := range v.Domains {
			if strings.EqualFold(domain, ZoneDomain+".") ||
				strings.HasSuffix(strings.ToLower(domain), strings.ToLower("."+ZoneDomain+".")) {
				return k
			}
		}
	}
	//  Else find default zone with .
	for k, v := range dnsProxy.zones {
		for _, ZoneDomain := range v.Domains {
			if ZoneDomain == "." {
				return k
			}
		}
	}
	// No zone found
	// log.Fatal("Failed to find zone for " + domain + ". Probably no zones with '.' domain.")
	return ""
}

func (dnsProxy *DNSProxy) getStatic(domain string) string {
	for k, v := range dnsProxy.static {
		if strings.EqualFold(k+".", domain) {
			return v
		}
	}
	return ""
}

func GetOutboundIP() (net.IP, error) {

	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	return localAddr.IP, nil
}

type LookupFunc func(string, *dns.Msg) (*dns.Msg, error)

func lookup(server string, m *dns.Msg) (*dns.Msg, error) {
	dnsClient := new(dns.Client)
	dnsClient.Net = "udp"
	response, _, err := dnsClient.Exchange(m, server)
	if err != nil {
		return nil, err
	}

	return response, nil
}

func (proxy *DNSProxy) MakeFakeIP(r net.IP, zoneID string) string {
	ip := proxy.zones[zoneID].Prefix
	if len(r) == net.IPv6len {
		ip[15] = r[15]
		ip[14] = r[14]
		ip[13] = r[13]
		ip[12] = r[12]
	} else {
		ip[15] = r[3]
		ip[14] = r[2]
		ip[13] = r[1]
		ip[12] = r[0]
	}
	return ip.String()
}

func ReversePTR(ptr string) (net.IP, error) {
	var ip net.IP
	if !strings.HasSuffix(ptr, ".in-addr.arpa.") && !strings.HasSuffix(ptr, ".ip6.arpa.") {
		return ip, fmt.Errorf("wrong ptr address in query %s", ptr)
	}
	s := strings.Split(ptr, ".")
	switch len(s) {
	case 7: // ipv4 in-addr arpa
		ip = make([]byte, net.IPv4len)
		for i, j := 0, net.IPv4len-1; i < 4; i, j = i+1, j-1 {
			a, err := strconv.ParseUint(s[i], 10, 8)
			if err != nil {
				return net.IP{}, err
			}
			ip[j] = byte(a)
		}
	case 35: // ipv6 ipv6 arpa
		ip = make([]byte, net.IPv6len)
		for i, j := 0, net.IPv6len-1; i < 32; i, j = i+2, j-1 {
			a, err := strconv.ParseUint(s[i], 16, 8)
			if err != nil {
				return net.IP{}, err
			}
			b, err := strconv.ParseUint(s[i+1], 16, 8)
			if err != nil {
				return net.IP{}, err
			}
			ip[j] = byte(b)<<4 | byte(a)
		}
	default: // wrong length
		return ip, fmt.Errorf("wrong PTR in query %s", ptr)
	}
	return ip, nil
}

func (proxy *DNSProxy) ReversePTR(ptr string, zoneID string) (ipv4 net.IP, err error) {
	var ip net.IP
	ip, err = ReversePTR(ptr)
	if err != nil {
		return
	}
	if len(ip) != net.IPv6len {
		err = fmt.Errorf("PTR is not IPv6")
	}
	for i := 0; i < 12; i++ {
		if ip[i] != proxy.zones[zoneID].Prefix[i] {
			err = fmt.Errorf("PTR doesn't have our prefix")
			return
		}
	}
	ipv4 = make([]byte, 4)
	ipv4[3] = ip[15]
	ipv4[2] = ip[14]
	ipv4[1] = ip[13]
	ipv4[0] = ip[12]
	return
}

func init() {
	_, yggnet, _ = net.ParseCIDR("200::/7")
}
