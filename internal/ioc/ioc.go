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
	reIPv6   = regexp.MustCompile(`(?:[0-9a-fA-F]{0,4}:){2,}[0-9a-fA-F]{0,4}`)
	reSHA256 = regexp.MustCompile(`\b[a-fA-F0-9]{64}\b`)
	reSHA1   = regexp.MustCompile(`\b[a-fA-F0-9]{40}\b`)
	reMD5    = regexp.MustCompile(`\b[a-fA-F0-9]{32}\b`)
	reDomain = regexp.MustCompile(`\b(?:[a-zA-Z0-9](?:[a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}\b`)
)

// Extract returns deduplicated indicators found in data, sorted by type then
// value. Hash precedence (sha256 > sha1 > md5) prevents a 64-hex string from
// also matching md5/sha1; emails/URLs are extracted before bare domains so the
// domain regex does not double-count their host parts.
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

	// Hashes, longest first so shorter hash regexes don't claim substrings.
	consumed := map[string]bool{} // hex strings already claimed as a longer hash
	for _, m := range reSHA256.FindAllString(s, -1) {
		add("sha256", m)
		consumed[m] = true
	}
	for _, m := range reSHA1.FindAllString(s, -1) {
		if !isSubOfConsumed(m, consumed) {
			add("sha1", m)
			consumed[m] = true
		}
	}
	for _, m := range reMD5.FindAllString(s, -1) {
		if !isSubOfConsumed(m, consumed) {
			add("md5", m)
		}
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

func isSubOfConsumed(m string, consumed map[string]bool) bool {
	for c := range consumed {
		if len(c) > len(m) && (indexOf(c, m) >= 0) {
			return true
		}
	}
	return false
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
