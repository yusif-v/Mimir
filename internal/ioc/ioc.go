// Package ioc extracts indicators of compromise from arbitrary text/bytes.
package ioc

import (
	"regexp"
	"sort"
)

type Indicator struct {
	Type  string
	Value string
}

var (
	reURL    = regexp.MustCompile(`\bhttps?://[^\s"'<>]+`)
	reEmail  = regexp.MustCompile(`\b[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}\b`)
	reIPv4   = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	reIPv6   = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){2,7}[0-9a-fA-F]{0,4}\b|\b[0-9a-fA-F]{1,4}(?::[0-9a-fA-F]{1,4})*::(?:[0-9a-fA-F]{1,4}(?::[0-9a-fA-F]{1,4})*)?\b`)
	reSHA256 = regexp.MustCompile(`\b[a-fA-F0-9]{64}\b`)
	reSHA1   = regexp.MustCompile(`\b[a-fA-F0-9]{40}\b`)
	reMD5    = regexp.MustCompile(`\b[a-fA-F0-9]{32}\b`)
	reDomain = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`)
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
	for _, m := range reIPv6.FindAllString(s, -1) {
		add("ipv6", m)
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
