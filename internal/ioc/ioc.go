// Package ioc extracts indicators of compromise from arbitrary text/bytes.
package ioc

import (
	"net"
	"regexp"
	"sort"
)

type Indicator struct {
	Type  string
	Value string
}

var (
	reURL   = regexp.MustCompile(`\bhttps?://[^\s"'<>]+`)
	reEmail = regexp.MustCompile(`\b[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}\b`)
	reIPv4  = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// reIPv6Candidates is intentionally loose: it matches colon-separated
	// hex groups (which covers the compressed `::` form, `fe80::1`, the
	// full 8-group form, and bare `::`). Every candidate is then validated
	// with net.ParseIP so time-like strings such as `06:19:54` or MAC
	// addresses like `00:1a:2b:3c:4d:5e` are never reported as IPv6.
	reIPv6Candidates = regexp.MustCompile(`(?:[0-9a-fA-F]{0,4}:){1,}[0-9a-fA-F]{0,4}`)
	reSHA256         = regexp.MustCompile(`\b[a-fA-F0-9]{64}\b`)
	reSHA1           = regexp.MustCompile(`\b[a-fA-F0-9]{40}\b`)
	reMD5            = regexp.MustCompile(`\b[a-fA-F0-9]{32}\b`)
	reDomain         = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`)
)

// Extract returns deduplicated indicators found in data, sorted by type then
// value. Emails/URLs are extracted before bare domains. The dedupe key is
// type+value, so URL hosts are also emitted as separate domain indicators.
func Extract(data []byte) []Indicator {
	s := string(data)
	seen := map[string]bool{}
	var out []Indicator
	add := func(typ, val string) {
		k := typ + "\x00" + val
		if !seen[k] {
			seen[k] = true
			out = append(out, Indicator{typ, val})
		}
	}

	// Hashes extracted independently; \b anchors prevent reSHA1/reMD5 from
	// matching a substring within a contiguous 64-hex SHA256 token.
	for _, m := range reSHA256.FindAllString(s, -1) {
		add("sha256", m)
	}
	for _, m := range reSHA1.FindAllString(s, -1) {
		add("sha1", m)
	}
	for _, m := range reMD5.FindAllString(s, -1) {
		add("md5", m)
	}

	for _, m := range reURL.FindAllString(s, -1) {
		add("url", m)
	}
	for _, m := range reEmail.FindAllString(s, -1) {
		add("email", m)
	}
	for _, m := range reIPv4.FindAllString(s, -1) {
		add("ipv4", m)
	}
	for _, m := range reIPv6Candidates.FindAllString(s, -1) {
		ip := net.ParseIP(m)
		if ip != nil && ip.To4() == nil {
			add("ipv6", m)
		}
	}
	for _, m := range reDomain.FindAllString(s, -1) {
		add("domain", m)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Value < out[j].Value
	})
	return out
}
