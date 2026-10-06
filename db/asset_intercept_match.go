package db

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Asset intercept matching. asset_intercept.go only stores the rules.
// Enabled rules are matched against a target asset's domain, IP, and URL.
// Agent tools (add_intent, insert_assets) call this before a trial or an asset insert, and a hit is denied.

// AssetInterceptKindLabel returns a readable kind label for agent-facing messages.
func AssetInterceptKindLabel(kind string) string {
	switch kind {
	case "exact_domain":
		return "Domain (exact)"
	case "exact_ip":
		return "IP (exact)"
	case "exact_url":
		return "URL (exact)"
	case "fuzzy_domain":
		return "Domain (fuzzy)"
	case "fuzzy_ip":
		return "IP (fuzzy)"
	case "fuzzy_url":
		return "URL (fuzzy)"
	case "cidr":
		return "CIDR range"
	}
	return kind
}

// Reason returns a readable hit reason, for example "matched asset intercept rule [Domain (fuzzy): .gov.cn] (note)".
func (r AssetInterceptRule) Reason() string {
	s := fmt.Sprintf("matched asset intercept rule [%s: %s]", AssetInterceptKindLabel(r.Kind), r.Pattern)
	if note := strings.TrimSpace(r.Note); note != "" {
		s += "(" + note + ")"
	}
	return s
}

// matchOne reports whether one enabled rule hits the given domain, IP, or URL candidates, and returns the matched value.
func matchOne(r AssetInterceptRule, domains, ips, urls []string) (string, bool) {
	p := strings.TrimSpace(r.Pattern)
	if p == "" {
		return "", false
	}
	switch r.Kind {
	case "exact_domain":
		for _, d := range domains {
			if strings.EqualFold(strings.TrimSpace(d), p) {
				return d, true
			}
		}
	case "exact_ip":
		for _, ip := range ips {
			if strings.TrimSpace(ip) == p {
				return ip, true
			}
		}
	case "exact_url":
		for _, u := range urls {
			if strings.TrimSpace(u) == p {
				return u, true
			}
		}
	case "fuzzy_domain":
		lp := strings.ToLower(p)
		for _, d := range domains {
			if d != "" && strings.Contains(strings.ToLower(d), lp) {
				return d, true
			}
		}
	case "fuzzy_ip":
		for _, ip := range ips {
			if ip != "" && strings.Contains(ip, p) {
				return ip, true
			}
		}
	case "fuzzy_url":
		lp := strings.ToLower(p)
		for _, u := range urls {
			if u != "" && strings.Contains(strings.ToLower(u), lp) {
				return u, true
			}
		}
	case "cidr":
		_, ipnet, err := net.ParseCIDR(p)
		if err != nil {
			return "", false
		}
		for _, ip := range ips {
			if pip := net.ParseIP(strings.TrimSpace(ip)); pip != nil && ipnet.Contains(pip) {
				return ip, true
			}
		}
	}
	return "", false
}

// MatchAssetInterceptRules returns the first enabled rule that hits the given domain, IP, or URL candidates, and the matched value.
// insert_assets uses it on raw input (an assetInputItem that is not stored yet).
func MatchAssetInterceptRules(rules []AssetInterceptRule, domains, ips, urls []string) (AssetInterceptRule, string, bool) {
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		if v, ok := matchOne(r, domains, ips, urls); ok {
			return r, v, true
		}
	}
	return AssetInterceptRule{}, "", false
}

// interceptCandidates extracts domain, IP, and URL candidates from a stored asset for intercept matching.
// The URL host is split out and classified, so a service asset that only has a URL can still hit a domain or IP rule.
func (a *Asset) interceptCandidates() (domains, ips, urls []string) {
	add := func(dst *[]string, s string) {
		if s = strings.TrimSpace(s); s != "" {
			*dst = append(*dst, s)
		}
	}
	add(&domains, a.Domain)
	add(&domains, a.RootDomain)
	for _, d := range a.BoundDomains {
		add(&domains, d)
	}
	add(&ips, a.IP)
	add(&urls, a.URL)
	if a.URL != "" {
		if u, err := url.Parse(a.URL); err == nil {
			if h := u.Hostname(); h != "" {
				if net.ParseIP(h) != nil {
					add(&ips, h)
				} else {
					add(&domains, h)
				}
			}
		}
	}
	return domains, ips, urls
}

// InterceptLabel returns a short asset label for agent-facing messages.
func (a *Asset) InterceptLabel() string {
	var target string
	switch {
	case a.Domain != "":
		target = a.Domain
	case a.URL != "":
		target = a.URL
	case a.IP != "":
		target = a.IP
	default:
		target = fmt.Sprintf("#%d", a.ID)
	}
	return fmt.Sprintf("Assets#%d[%s] %s", a.ID, a.Type, target)
}

// hasEnabledRule reports whether any rule in the set is enabled.
func hasEnabledRule(rules []AssetInterceptRule) bool {
	for _, r := range rules {
		if r.Enabled {
			return true
		}
	}
	return false
}

// AssetGateDecision is the block-then-allow gate's decision for one set of candidates.
type AssetGateDecision struct {
	Allowed bool
	Reason  string // Reasons for rejection (excluding asset identification));Allowed=true Time is empty
}

// EvaluateAssetGate runs the task-level gate:
//  1. A hit on any enabled blockRules rejects (intercept reason).
//  2. Otherwise, if allowRules has an enabled entry and none hit, reject (not in the allowlist).
//  3. Otherwise allow.
//
// When allowRules is empty or has no enabled entry, the allow gate is off
// (no allowlist, everything is allowed). That avoids blocking every asset
// just because no allow rule was configured.
func EvaluateAssetGate(blockRules, allowRules []AssetInterceptRule, domains, ips, urls []string) AssetGateDecision {
	if rule, _, ok := MatchAssetInterceptRules(blockRules, domains, ips, urls); ok {
		return AssetGateDecision{Allowed: false, Reason: rule.Reason()}
	}
	if hasEnabledRule(allowRules) {
		if _, _, ok := MatchAssetInterceptRules(allowRules, domains, ips, urls); !ok {
			return AssetGateDecision{Allowed: false, Reason: "outside the task allowlist; testing is not allowed"}
		}
	}
	return AssetGateDecision{Allowed: true}
}

// AssetInterceptHit describes an asset the gate rejected (an intercept hit, or not in the allowlist).
type AssetInterceptHit struct {
	Asset  *Asset
	Reason string // Readable Reasons
}

// Describe returns a readable line: asset label plus reason.
func (h AssetInterceptHit) Describe() string {
	return fmt.Sprintf("%s → %s", h.Asset.InterceptLabel(), h.Reason)
}

// ListAssetInterceptRules forwards the *DB method of the same name so a caller that only holds an AssetStore (an agent tool) can read the rules.
func (s *AssetStore) ListAssetInterceptRules() ([]AssetInterceptRule, error) {
	return s.db.ListAssetInterceptRules()
}

// CheckAssetsIntercept loads assets by id, runs the block-then-allow gate on each, and returns every rejected asset.
// Block rules are the global set union the task-level block rules. Allow rules are the task-level allow rules for this task only.
// An empty id list returns immediately. It uses the global GetByIDs (not filtered by task scope) so scope cannot weaken interception.
func (s *AssetStore) CheckAssetsIntercept(taskID int64, ids []int64) ([]AssetInterceptHit, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	blockRules, err := s.db.ListAssetInterceptRules()
	if err != nil {
		return nil, err
	}
	var allowRules []AssetInterceptRule
	if taskID > 0 {
		tb, ta, err := s.TaskInterceptRulesSplit(taskID)
		if err != nil {
			return nil, err
		}
		blockRules = append(blockRules, tb...)
		allowRules = ta
	}
	// No block rules and no enabled allow rules: nothing to decide, allow everything.
	if len(blockRules) == 0 && !hasEnabledRule(allowRules) {
		return nil, nil
	}
	assets, err := s.GetByIDs(ids)
	if err != nil {
		return nil, err
	}
	var hits []AssetInterceptHit
	for _, a := range assets {
		domains, ips, urls := a.interceptCandidates()
		if d := EvaluateAssetGate(blockRules, allowRules, domains, ips, urls); !d.Allowed {
			hits = append(hits, AssetInterceptHit{Asset: a, Reason: d.Reason})
		}
	}
	return hits, nil
}
