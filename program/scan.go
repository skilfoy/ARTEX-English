package program

import "strings"

// ScanText rejects secret-shaped text before it is stored on the desk.
// It does not decide whether a sentence is an instruction.
func ScanText(text string) (bool, string) {
	if strings.Contains(text, "AKIA") || strings.Contains(text, "BEGIN PRIVATE KEY") || strings.Contains(text, "BEGIN OPENSSH PRIVATE KEY") {
		return false, "secret material"
	}
	lower := strings.ToLower(text)
	for _, key := range []string{"password=", "token=", "api_key=", "secret="} {
		if strings.Contains(lower, key) {
			return false, "credential assignment"
		}
	}
	return true, ""
}
