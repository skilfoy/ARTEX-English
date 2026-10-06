package notify

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
)

// Channel It's a adapter for a notification channel. Achieving the necessary**No status**:The same example will get multiple channels.
// Configure and re-assign from all sources cfg Parameter In.
type Channel interface {
	// Kind Return channel type identification, consistent with the key of the registration form.
	Kind() string
	// Validate Call when saving the configuration, verify the field and format required. The returned error will be displayed directly to
	// Configurer, so the file has to be specified.[Which field is missing]Not general.[Configuration Invalid].
	Validate(cfg map[string]any) error
	// Send Can not open message**Number of entries actually delivered**and error.
	//
	// Why return bars: each platform has a maximum message length, and aggregate messages are stopped when the whole batch is not loaded.
	// If the caller unconditionally marked the entire batch as delivered, those entries that were intercepted disappeared.——It's not in the news.,
	// The delivery history has also shown success, and nowhere has a loophole ever been found. Back kept After,
	// Caller only before mark kept Article, remaining to be submitted.
	//
	// Returns error means delivery failed, where *PermanentError It means you shouldn't try again..
	// When failed kept No sense. The caller should ignore it..
	Send(ctx context.Context, cfg map[string]any, m Message) (int, error)
	// DefaultRatePerMin Return to the channel officially recommended per minute ceiling as an example of a new channel
	// . The default limit value. Back 0 Means no known limit.
	DefaultRatePerMin() int
	// SecretKeys Returns a documented keyname in the channel configuration.API The value of these keys will be masked when it turns around.,
	// The mask value is kept in the library when the update is received. Only if you know what fields you're looking for.
	// (The whole business. Webhook The address is the proof, and the nails are the only one. secret),
	// So this knowledge has to come from a source, not from the top..
	SecretKeys() []string
	// DestinationKeys Return to the channel configuration[Where's the message going?]Keyname.
	//
	// With SecretKeys It's also about security: target address and proof are two separate fields.,
	// If allowed[Only change of address, keep as it is.],Anyone who can change the configuration can get the real evidence in the library.
	// Send them to servers under their control, and the code of channel configuration is completely meaningless..
	// For more details. PrepareConfigUpdate.
	DestinationKeys() []string
}

// registry is the channel registration form. It's meant to be visible, not init() Self-registered:[What channels?]
// It can be seen in one place, and the new channels will expose the missing during the compilation, not by running side effects..
var registry = map[string]Channel{
	KindDingTalk: dingTalkChannel{},
	KindFeishu:   feishuChannel{},
	KindWeCom:    weComChannel{},
	KindWebhook:  webhookChannel{},
	KindTelegram: telegramChannel{},
	KindEmail:    emailChannel{},
}

// Get By type of access.
func Get(kind string) (Channel, bool) {
	c, ok := registry[kind]
	return c, ok
}

// ValidKind Report kind Type of channel supported.
func ValidKind(kind string) bool {
	_, ok := registry[kind]
	return ok
}

// Kinds Returns all supported channel types, in dictionaries order (for UI Pull steady.).
func Kinds() []string {
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PermanentError Mark a delivery that should not be retried failed: error of proof, object rejection, request illegal, etc..
// Retry only instantaneous failure (network shaking, limit flow, opposite end) 5xx)Meaningful; reticence of permanent failure
// It's not going to work, it's not going to work. It's not going to work..
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// Permanent handle err Mark as permanent failure.err for nil Back nil,
// It's easy to write. `return Permanent(someCheck())`.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}

// IsPermanent Report err Is there a permanent failure mark on the chain?.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// ---- Configure Readhelper ----
//
// Channel configuration from database JSONB Column, through encoding/json After the inverse sequence, map[string]any,
// The value is always... float64,The array is... []any.Here. helper Harmonize this layer and tolerate users
// at UI Type deviation due to emptying (e.g. filling port into string)).

// cfgString Take string configurations and always cut them out.——Copying paste from web forms is easy to carry.
func cfgString(cfg map[string]any, key string) string {
	v, ok := cfg[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// cfgInt Take integer configuration, compatible float64(JSON Default) and string sources.
func cfgInt(cfg map[string]any, key string) int {
	switch v := cfg[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// cfgBool Take Boolean Configuration, Compatible String "true"/"1".
func cfgBool(cfg map[string]any, key string) bool {
	switch v := cfg[key].(type) {
	case bool:
		return v
	case string:
		s := strings.ToLower(strings.TrimSpace(v))
		return s == "true" || s == "1" || s == "yes"
	default:
		return false
	}
}

// cfgStrings Take string array configurations, customize blanks and discard empty strings.
func cfgStrings(cfg map[string]any, key string) []string {
	raw, ok := cfg[key].([]any)
	if !ok {
		// Accepts a single string to facilitate the submission of a form at a single value.
		if s := cfgString(cfg, key); s != "" {
			return []string{s}
		}
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// cfgMap Take string map configuration (e.g. custom) HTTP Head) , keys are all spaced and empty.
func cfgMap(cfg map[string]any, key string) map[string]string {
	raw, ok := cfg[key].(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		s, ok := v.(string)
		if !ok {
			continue
		}
		out[k] = s
	}
	return out
}
