package db

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// Matching asset interception rules/Executive Level.asset_intercept.go Only the rules are stored.
// [Target assets]Domain Name/IP/URL Matches the rules that are enabled. For agent Tools(add_intent,
// insert_assets)I'm trying. / Call before inserting the asset, and hit is denied.

// AssetInterceptKindLabel Return kind Other Organiser agent Note message.
func AssetInterceptKindLabel(kind string) string {
	switch kind {
	case "exact_domain":
		return "Domain name(Congruent)"
	case "exact_ip":
		return "IP(Congruent)"
	case "exact_url":
		return "URL(Congruent)"
	case "fuzzy_domain":
		return "Domain name(Blurred)"
	case "fuzzy_ip":
		return "IP(Blurred)"
	case "fuzzy_url":
		return "URL(Blurred)"
	case "cidr":
		return "CIDR Network segment"
	}
	return kind
}

// Reason Return a readable cause of impact, such as the hit asset interception rules [Domain name(Blurred): .gov.cn](Remarks).
func (r AssetInterceptRule) Reason() string {
	s := fmt.Sprintf("Rules for the interception of hit assets [%s: %s]", AssetInterceptKindLabel(r.Kind), r.Pattern)
	if note := strings.TrimSpace(r.Note); note != "" {
		s += "(" + note + ")"
	}
	return s
}

// matchOne Determines whether the single enabled rule hits the given domain name/IP/URL Candidate string, return the hit value.
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

// MatchAssetInterceptRules Returns first hit given domain name/IP/URL Enable rules for candidate strings, and the specific values of hits.
// Supply insert_assets Original input (not yet in library) assetInputItem)match.
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

// interceptCandidates Extract a domain name that has been stored to intercept matching assets/IP/URL Candidate string.
// URL of host They'll be removed and classified, so...[Only URL]Service-type assets can also be used as domain names/IP Rules hit..
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

// InterceptLabel Short identification of returned assets to be used for agent Note message.
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

// hasEnabledRule Determines if there are any enabling rules in the rule book.
func hasEnabledRule(rules []AssetInterceptRule) bool {
	for _, r := range rules {
		if r.Enabled {
			return true
		}
	}
	return false
}

// AssetGateDecision Yes[Intercept and allow.]The outcome of the gate against a group of candidates..
type AssetGateDecision struct {
	Allowed bool
	Reason  string // Reasons for rejection (excluding asset identification));Allowed=true Time is empty
}

// EvaluateAssetGate Mission level gate.:
//  1. Hit any enabled blockRules → Rejection (cause of interception)).
//  2. If not, allowRules Enabled and missed → Rejection (not permitted)).
//  3. Or you'll be released..
//
// allowRules Empty/Allow gates not to work when no entries are enabled (i.e., no white list enabled, all released)),
// Avoid[No allowed rules configured]Block all assets..
func EvaluateAssetGate(blockRules, allowRules []AssetInterceptRule, domains, ips, urls []string) AssetGateDecision {
	if rule, _, ok := MatchAssetInterceptRules(blockRules, domains, ips, urls); ok {
		return AssetGateDecision{Allowed: false, Reason: rule.Reason()}
	}
	if hasEnabledRule(allowRules) {
		if _, _, ok := MatchAssetInterceptRules(allowRules, domains, ips, urls); !ok {
			return AssetGateDecision{Allowed: false, Reason: "Not allowed on mission(whitelist)Within range, no tests allowed"}
		}
	}
	return AssetGateDecision{Allowed: true}
}

// AssetInterceptHit Description of an asset that was rejected by the gate (interception hit or not permitted)).
type AssetInterceptHit struct {
	Asset  *Asset
	Reason string // Readable Reasons
}

// Describe Returns a readable description: asset information + Reasons.
func (h AssetInterceptHit) Describe() string {
	return fmt.Sprintf("%s → %s", h.Asset.InterceptLabel(), h.Reason)
}

// ListAssetInterceptRules Yes *DB The transmission of the same-named method, to be held only AssetStore Caller
// (As agent You can read the rules..
func (s *AssetStore) ListAssetInterceptRules() ([]AssetInterceptRule, error) {
	return s.db.ListAssetInterceptRules()
}

// CheckAssetsIntercept Press id Load assets, one by one[Intercept and allow.]The gate says back to everything.
// Rejected assets. Interception rules = Global ∪ Task level block;Allow rules = Task level allow(This task only).
// None id . Use global GetByIDs(It's not filtered by the mission. scope Weakness.
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
	// There are no rules of interception, no rules of access. → You don't have to judge. Release them all..
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
