package program

import "strings"

const endMarker = "<<<END UNTRUSTED>>>"

// WrapUntrusted fences text that came from a tool or a worker before that
// text is placed back into a prompt. The body is data. Markers inside the
// body are broken so the body cannot close the fence early.
func WrapUntrusted(text, label string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "<<<UNTRUSTED ") {
		return text
	}
	label = cleanLabel(label)
	return "<<<UNTRUSTED " + label + ">>>\n" + breakMarkers(text) + "\n" + endMarker
}

func cleanLabel(label string) string {
	label = breakMarkers(label)
	label = strings.Map(func(r rune) rune {
		if r == '<' || r == '>' || r == '\n' || r == '\r' {
			return '-'
		}
		return r
	}, label)
	if label == "" {
		return "TEXT"
	}
	return label
}

func breakMarkers(text string) string {
	text = strings.ReplaceAll(text, endMarker, "‹‹‹END UNTRUSTED›››")
	return strings.ReplaceAll(text, "<<<", "‹‹‹")
}
