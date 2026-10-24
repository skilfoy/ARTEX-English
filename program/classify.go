package program

import "strings"

// Categories are property names, not procedures. A finding is labeled with
// one of them so a report says which property failed.
const (
	CatSpoofing        = "spoofing"
	CatTampering       = "tampering"
	CatRepudiation     = "repudiation"
	CatInfoDisclosure  = "info-disclosure"
	CatDenialOfService = "denial-of-service"
	CatPrivilege       = "privilege-escalation"
	CatAgentic         = "agentic"
	CatUnclassified    = "unclassified"
)

// Classify maps a finding title and class name onto one category. It matches
// words already used by the operator. It does not invent a next step.
func Classify(name, vulnClass string) string {
	text := strings.ToLower(name + " " + vulnClass)
	switch {
	case hasAny(text, "agent", "prompt", "tool-call", "mcp"):
		return CatAgentic
	case hasAny(text, "spoof", "authent", "session", "credential"):
		return CatSpoofing
	case hasAny(text, "tamper", "integrit"):
		return CatTampering
	case hasAny(text, "repudiat", "audit log", "logging"):
		return CatRepudiation
	case hasAny(text, "disclos", "expos", "leak"):
		return CatInfoDisclosure
	case hasAny(text, "denial", "availability"):
		return CatDenialOfService
	case hasAny(text, "privilege", "authoriz"):
		return CatPrivilege
	default:
		return CatUnclassified
	}
}

func hasAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}
