package notify

import (
	"errors"
	"strings"
	"testing"
)

func TestMaskedValueHidesBodyButKeepsTailHint(t *testing.T) {
	const secret = "https://oapi.dingtalk.com/robot/send?access_token=abcdef123456"
	got := MaskedValue(secret)
	if strings.Contains(got, "abcdef123456") {
		t.Fatalf("The masked value leaked the full evidence.: %q", got)
	}
	if strings.Contains(got, "oapi.dingtalk.com") {
		t.Fatalf("The mask value should not expose the subject: %q", got)
	}
	// End 6 If you want to keep it, the user will recognize which machine. People.
	if !strings.HasSuffix(got, "123456") {
		t.Fatalf("The end should be retained 6 Bits as identifiers: %q", got)
	}
	if !IsMasked(got) {
		t.Fatalf("The mask must be able to be used. IsMasked Identification: %q", got)
	}
}

func TestMaskedValueShortSecretGivesNoHint(t *testing.T) {
	// Short certificates, if they're exposed. 6 It's like exposing the whole evidence..
	for _, s := range []string{"abc", "abcdef", ""} {
		got := MaskedValue(s)
		if got != MaskedPrefix {
			t.Fatalf("Length %d The proof should not give a tailtip. %q", len(s), got)
		}
		if s != "" && strings.Contains(got, s) {
			t.Fatalf("The mask value contains the original value: %q", got)
		}
	}
}

func TestMaskConfigMasksOnlySecrets(t *testing.T) {
	cfg := map[string]any{
		"webhook": "https://example.com/hook?token=SECRETVALUE",
		"secret":  "SECtest123456",
		"port":    float64(587),
		"host":    "smtp.example.com",
	}
	masked := MaskConfig(KindDingTalk, cfg)
	for _, k := range []string{"webhook", "secret"} {
		s, _ := masked[k].(string)
		if !IsMasked(s) {
			t.Errorf("%s Should be masked. Got it. %q", k, s)
		}
	}
	// The non-substantiation field must be retained as it is, otherwise UI Unable to display.
	if masked["port"] != float64(587) {
		t.Errorf("Non-supported fields port No change: %v", masked["port"])
	}
}

func TestMaskConfigUnknownKindReturnsEmpty(t *testing.T) {
	// If you can't identify the type of channel, you'd rather have it. UI Show empty configuration and don't throw back the original content that may contain the proof.
	got := MaskConfig("nope", map[string]any{"webhook": "https://x/y?token=LEAK"})
	if len(got) != 0 {
		t.Fatalf("Unknown channel type should return empty configuration and get %v", got)
	}
}

func TestMaskConfigDoesNotMutateInput(t *testing.T) {
	// The mask is a display of behavior..
	cfg := map[string]any{"webhook": "https://example.com/hook", "secret": "SECtest123456"}
	_ = MaskConfig(KindDingTalk, cfg)
	if IsMasked(cfg["secret"].(string)) {
		t.Fatal("MaskConfig Modified reference, resulting in true evidence overwhelmed by code value")
	}
}

func TestMergeConfigKeepsStoredOnMaskedIncoming(t *testing.T) {
	stored := map[string]any{"webhook": "https://real/hook", "secret": "REALSECRET", "method": "POST"}
	// The users have changed. method,Browser submitted a mask value+New method.
	incoming := map[string]any{
		"webhook": MaskedValue("https://real/hook"),
		"secret":  MaskedValue("REALSECRET"),
		"method":  "PUT",
	}
	got := MergeConfig(stored, incoming)
	if got["webhook"] != "https://real/hook" || got["secret"] != "REALSECRET" {
		t.Fatalf("The mask field should retain the original value in the library and get %v", got)
	}
	if got["method"] != "PUT" {
		t.Fatalf("The modified field should become effective and receive %v", got["method"])
	}
}

func TestMergeConfigEmptyStringClears(t *testing.T) {
	stored := map[string]any{"webhook": "https://real/hook", "secret": "REALSECRET"}
	got := MergeConfig(stored, map[string]any{"secret": ""})
	if _, ok := got["secret"]; ok {
		t.Fatalf("Empty string should empty the field, get %v", got)
	}
	// Keep fields not mentioned (locally updated semantics)).
	if got["webhook"] != "https://real/hook" {
		t.Fatalf("Fields not mentioned should be retained and obtained %v", got)
	}
}

func TestMergeConfigKeepsUnmentionedStoredKeys(t *testing.T) {
	stored := map[string]any{"host": "smtp.example.com", "port": float64(587), "password": "pw"}
	got := MergeConfig(stored, map[string]any{"port": float64(465)})
	if got["host"] != "smtp.example.com" || got["password"] != "pw" {
		t.Fatalf("Fields not mentioned should be retained and obtained %v", got)
	}
	if got["port"] != float64(465) {
		t.Fatalf("Fields already mentioned should be updated and obtained %v", got["port"])
	}
}

// TestPrepareConfigUpdateBlocksDestinationSwap It's the most important security variable in this package.:
// **You can't take the old papers with you.**.
//
// These examples are the input of attack shapes.),
// instead of[The correct input of defense logic]——If it's only the latter, it's the defense that doesn't work. All green..
func TestPrepareConfigUpdateBlocksDestinationSwap(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		stored   map[string]any
		incoming map[string]any
		// wantMissing It's a certificate key expected to be called..
		wantMissing string
	}{
		{
			name: "General Webhook I'd like to change my address. Authorization head",
			kind: KindWebhook,
			stored: map[string]any{
				"url":     "https://legit.example.com/hook",
				"headers": map[string]any{"Authorization": "Bearer REAL-TOKEN"},
			},
			incoming:    map[string]any{"url": "https://attacker.tld/c"},
			wantMissing: "headers",
		},
		{
			name:        "Telegram Change base_url I want to. Bot Token Send to Your End",
			kind:        KindTelegram,
			stored:      map[string]any{"bot_token": "123456:REAL", "chat_id": "1", "base_url": "https://api.telegram.org"},
			incoming:    map[string]any{"base_url": "https://attacker.tld"},
			wantMissing: "bot_token",
		},
		{
			name:        "Mail Change SMTP The host wants to hand over the password.",
			kind:        KindEmail,
			stored:      map[string]any{"host": "smtp.corp.com", "port": 587, "password": "REALPW", "from": "a@b.c", "to": []any{"d@e.f"}},
			incoming:    map[string]any{"host": "smtp.attacker.tld"},
			wantMissing: "password",
		},
		{
			name:        "Mail off. TLS We have to rephrase the password.",
			kind:        KindEmail,
			stored:      map[string]any{"host": "smtp.corp.com", "port": 587, "tls": false, "password": "REALPW", "from": "a@b.c", "to": []any{"d@e.f"}},
			incoming:    map[string]any{"tls": true},
			wantMissing: "password",
		},
		{
			// Mask value = [Follow old evidence.],In the same context as the change of address, you must refuse..
			name:        "Back to cover. + New Address",
			kind:        KindTelegram,
			stored:      map[string]any{"bot_token": "123456:REAL", "chat_id": "1", "base_url": "https://api.telegram.org"},
			incoming:    map[string]any{"base_url": "https://attacker.tld", "bot_token": MaskedValue("123456:REAL")},
			wantMissing: "bot_token",
		},
		{
			name:        "The nails. Webhook You want to use a signer key?",
			kind:        KindDingTalk,
			stored:      map[string]any{"webhook": "https://oapi.dingtalk.com/robot/send?access_token=OLD", "secret": "REALSEC"},
			incoming:    map[string]any{"webhook": "https://attacker.tld/hook"},
			wantMissing: "secret",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			merged, err := PrepareConfigUpdate(tc.kind, tc.stored, tc.incoming)
			if err == nil {
				t.Fatalf("Change of address without a new statement should be rejected; configured %v", merged)
			}
			var target *ErrDestinationChangedWithoutCredentials
			if !errors.As(err, &target) {
				t.Fatalf("Should return a specific type of error so that the interface gives an actionable hint and gets %T: %v", err, err)
			}
			found := false
			for _, m := range target.Missing {
				if m == tc.wantMissing {
					found = true
				}
			}
			if !found {
				t.Fatalf("Noted missing certificate keys %q,get %v", tc.wantMissing, target.Missing)
			}
			// The error message should guide the operator how to fix it..
			if !strings.Contains(err.Error(), tc.wantMissing) {
				t.Errorf("Error message should be mentioned %q: %v", tc.wantMissing, err)
			}
		})
	}
}

// TestPrepareConfigUpdateAllowsLegitimateEdits Inverse use example: normal editing cannot be delayed,
// Otherwise, the defense will be...[Too annoying.]They've been bypassed or deleted..
func TestPrepareConfigUpdateAllowsLegitimateEdits(t *testing.T) {
	cases := []struct {
		name     string
		kind     string
		stored   map[string]any
		incoming map[string]any
	}{
		{
			name:     "Change name only (configure original returns))",
			kind:     KindWebhook,
			stored:   map[string]any{"url": "https://legit.example.com/hook", "headers": map[string]any{"Authorization": "Bearer REAL"}},
			incoming: map[string]any{"url": MaskedValue("https://legit.example.com/hook")},
		},
		{
			name:     "Just change the way you asked. The address and the proof.",
			kind:     KindWebhook,
			stored:   map[string]any{"url": "https://legit.example.com/hook", "method": "POST"},
			incoming: map[string]any{"method": "PUT"},
		},
		{
			name:     "Change address and**At the same time**Give the new papers.",
			kind:     KindWebhook,
			stored:   map[string]any{"url": "https://old.example.com/hook", "headers": map[string]any{"Authorization": "Bearer OLD"}},
			incoming: map[string]any{"url": "https://new.example.com/hook", "headers": map[string]any{"Authorization": "Bearer NEW"}},
		},
		{
			name:     "Change address and express statement no longer require proof",
			kind:     KindWebhook,
			stored:   map[string]any{"url": "https://old.example.com/hook", "headers": map[string]any{"Authorization": "Bearer OLD"}},
			incoming: map[string]any{"url": "https://new.example.com/hook", "headers": ""},
		},
		{
			name:     "Telegram Change chat_id(Not destination.)",
			kind:     KindTelegram,
			stored:   map[string]any{"bot_token": "t", "chat_id": "1", "base_url": "https://api.telegram.org"},
			incoming: map[string]any{"chat_id": "-100200"},
		},
		{
			name:     "Mail to recipient (not destination))",
			kind:     KindEmail,
			stored:   map[string]any{"host": "smtp.corp.com", "port": 587, "password": "PW", "from": "a@b.c", "to": []any{"x@y.z"}},
			incoming: map[string]any{"to": []any{"new@y.z"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			merged, err := PrepareConfigUpdate(tc.kind, tc.stored, tc.incoming)
			if err != nil {
				t.Fatalf("Legitimate editor blocked.: %v", err)
			}
			if merged == nil {
				t.Fatal("Should return the merger result")
			}
		})
	}
}

// TestPrepareConfigUpdatePortTypeTolerance Override an easily miscalculated detail:
// The port for the front-end submission is JSON number(float64),So does Curly. float64,
// But there may be different types of values (e.g. int vs float64).Use == It's better.[No change.]Agreed.[Changed.],
// So it pops up to a user whose name is changed.[Please refill the password.]——Fake alarms make people no longer trust this shield..
func TestPrepareConfigUpdatePortTypeTolerance(t *testing.T) {
	stored := map[string]any{"host": "smtp.corp.com", "port": float64(587), "password": "PW"}
	// Same port, to int Form of submission.
	if _, err := PrepareConfigUpdate(KindEmail, stored, map[string]any{"port": 587}); err != nil {
		t.Fatalf("Port value equal (of different types only) should not be awarded as an address change: %v", err)
	}
	// If you change the port, you have to stop it..
	if _, err := PrepareConfigUpdate(KindEmail, stored, map[string]any{"port": 25}); err == nil {
		t.Fatal("Port changes should be stopped.")
	}
}

// TestPrepareConfigUpdateSurvivesRepeatedSaveWithBlankDestination override[It's empty.
// Destination field]This path.:Telegram of base_url Leave space for official use API Address.
//
// Once here, the channel failed to be permanently preserved from the second.:
//
//	Save in New TimeCurrent base_url:""(Create a path directly to the frontend config,Not leaving MergeConfig)
//	→ First Save,MergeConfig Use empty strings as visible empties,delete Drop the button.
//	→ Second Save,incoming Still. "",And stored There's no such key.[The address changed.]
//	→ bot_token It's a mask echo. → 400[Target address changed. Please refill the certificate field at the same time]
//
// Users haven't changed anything, but they won't be able to keep it until they repaint it. Bot Token.
func TestPrepareConfigUpdateSurvivesRepeatedSaveWithBlankDestination(t *testing.T) {
	stored := map[string]any{"bot_token": "123:ABC", "chat_id": "-100", "base_url": ""}

	// Frontend buildConfig() A value is submitted for each field definition of the channel: a backfilling mask based on the document,
	// Empty text box submits an empty string. It reproduces its output in its entirety, rather than just submitting it.[Changed Key].
	submit := func() map[string]any {
		return map[string]any{
			"bot_token": MaskedValue("123:ABC"),
			"chat_id":   "-100",
			"base_url":  "",
		}
	}

	// First saving: only channel names changed,config Reply as received.
	merged, err := PrepareConfigUpdate(KindTelegram, stored, submit())
	if err != nil {
		t.Fatalf("The first saving was delayed.: %v", err)
	}
	if _, ok := merged["base_url"]; ok {
		t.Fatal("The premise has changed: empty strings should be MergeConfig Delete——That's exactly what this is about.[After the key disappears]That step.")
	}

	// Second saving: submission is fully consistent with last time and the user has not changed anything..
	merged2, err := PrepareConfigUpdate(KindTelegram, merged, submit())
	if err != nil {
		t.Fatalf("The second preservation was delayed.): %v", err)
	}
	// Third time, no.[Just once.]It's stable..
	if _, err := PrepareConfigUpdate(KindTelegram, merged2, submit()); err != nil {
		t.Fatalf("The third saving was delayed.: %v", err)
	}
	// The evidence has to be kept all the way down, and it's not cleared out by empty logic..
	if got := merged2["bot_token"]; got != "123:ABC" {
		t.Fatalf("Bot Token The original value should be maintained and obtained %v", got)
	}
}

// TestPrepareConfigUpdateStillGuardsBlankDestinationChanges It's a match for the previous example.
// 'Accusations: Zero strings with[Key does not exist]Consider it equal.,**I can't.**We're not gonna let the real address change..
// Both directions are real evidence of an outside route.——Telegram of Bot Token Walk in URL Path,
// Changed. base_url It's like... Token Send new address.
func TestPrepareConfigUpdateStillGuardsBlankDestinationChanges(t *testing.T) {
	// Direction I: From[Empty](To build your own address.
	official := map[string]any{"bot_token": "123:ABC", "chat_id": "-100"}
	if _, err := PrepareConfigUpdate(KindTelegram, official, map[string]any{
		"bot_token": MaskedValue("123:ABC"),
		"base_url":  "https://tg-proxy.attacker.tld",
	}); err == nil {
		t.Fatal("The change of address from the official address to the self-built address must be refilled. Token")
	}

	// Direction two: Empty your own address.(= Change to official. API)It's also a change of address..
	proxied := map[string]any{"bot_token": "123:ABC", "base_url": "https://proxy.internal/bot"}
	if _, err := PrepareConfigUpdate(KindTelegram, proxied, map[string]any{
		"bot_token": MaskedValue("123:ABC"),
		"base_url":  "",
	}); err == nil {
		t.Fatal("Clear your own address. API)It's also a change of address that has to be rewritten. Token")
	}
}

func TestDestinationKeysDeclaredForEveryKind(t *testing.T) {
	// With SecretKeys Same: If the channel forgets the stated destination key,PrepareConfigUpdate I can't protect it..
	for kind, ch := range registry {
		if len(ch.DestinationKeys()) == 0 {
			t.Errorf("Channel %s Undeclared destination key. The security against the change of address with proof is ineffective.", kind)
		}
		if len(ch.SecretKeys()) == 0 {
			t.Errorf("Channel %s Undeclared Key", kind)
		}
	}
}

func TestSecretKeysDeclaredForEveryKind(t *testing.T) {
	// The compiler has forced every channel to do it. SecretKeys,Let's check it again.[There's no access to the mask.
	// Hand over the paper.]——Returning to the channels of the empty slice means that its evidence will be clearly reflected in the browser..
	expect := map[string]bool{
		KindDingTalk: true, KindFeishu: true, KindWeCom: true,
		KindWebhook: true, KindTelegram: true, KindEmail: true,
	}
	for kind, ch := range registry {
		if !expect[kind] {
			t.Errorf("Channel %s The mask is not expected to be registered in the test", kind)
			continue
		}
		if len(ch.SecretKeys()) == 0 {
			t.Errorf("Channel %s No proof fields are declared, and their configuration will be explicitly repeated", kind)
		}
	}
}

// TestPrepareConfigUpdateRejectsMaskedInContainer Override one of the words identified in the audit:
// Put the mask in.**Nonstring**Structure (e.g. webhook.headers When it's an object),
// MergeConfig Only[String with prefix]It's a cover, so it's a volume. "__masked__" It's going to be considered
// Real head value in library——Follow-up silence lapsed and nothing was reported. Wrong..
func TestPrepareConfigUpdateRejectsMaskedInContainer(t *testing.T) {
	stored := map[string]any{
		"url":     "https://legit.example.com/hook",
		"headers": map[string]any{"Authorization": "Bearer REAL"},
	}
	// Inside the object with a masked sentry..
	incoming := map[string]any{
		"headers": map[string]any{"Authorization": MaskedPrefix},
	}
	if _, err := PrepareConfigUpdate(KindWebhook, stored, incoming); err == nil {
		t.Fatal("The inside of the structure should be turned down with a masked sentry. Library)")
	}
	// Whole object (real new value) accepted as usual.
	ok := map[string]any{"headers": map[string]any{"Authorization": "Bearer NEW"}}
	if _, err := PrepareConfigUpdate(KindWebhook, stored, ok); err != nil {
		t.Fatalf("Regular submission of new requests should not be blocked.: %v", err)
	}
}
