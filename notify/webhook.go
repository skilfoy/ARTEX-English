package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// webhookChannel General Webhook Adapter: User-specific URL,Methodology, head of request JSON Templates.
// Its existence makes it unnecessary. Slack / Mattermost / Discord / One for each self-builder.——
// Those platforms can be covered by a matching template..
type webhookChannel struct{}

func (webhookChannel) Kind() string { return KindWebhook }

// General Webhook No official restrictions. Return. 0 This means that the default is open-ended and is determined by the user by reciprocal capacity.
func (webhookChannel) DefaultRatePerMin() int { return 0 }

// Mask url With headers:Target addresses themselves are often carried token,There's usually a forensic certificate in the definition.,
// Both will appear in the interface, so they'll both be blocked..
// The cost is to refill the entire group if the editor is to change one of the heads. (The mask value is to be interpreted as[Keep original value])——
// It's a deliberate trade-off: I'd rather fill it out more than show it back to the browser..
func (webhookChannel) SecretKeys() []string { return []string{"url", "headers"} }

// The destination is... url.Change url It's time to say it again. headers —— Or the original. Authorization head
// It'll be sent to a new address. That's the main path around the mask..
func (webhookChannel) DestinationKeys() []string { return []string{"url"} }

// webhookDefaultTemplate It's a bottom request without a template: a white one JSON Structure,
// Overwrite Most[Take one. JSON Library]Self-built Receiver End.
const webhookDefaultTemplate = `{
  "title": {{json .Title}},
  "batch": {{.Batch}},
  "count": {{.Count}},
  "items": [
{{- range $i, $it := .Items}}
{{- if $i}},{{end}}
    {
      "finding_id": {{$it.FindingID}},
      "name": {{json $it.Name}},
      "vulnclass": {{json $it.VulnClass}},
      "severity": {{json $it.Severity}},
      "summary": {{json $it.Summary}},
      "assets": {{json $it.Assets}},
      "detail_url": {{json $it.DetailURL}}
    }
{{- end}}
  ]
}`

// webhookTemplateData It's the context of exposure to the user template..
type webhookTemplateData struct {
	Title   string
	Batch   bool
	Count   int
	Items   []webhookItem
	HomeURL string
	// SentAt It's time to deliver.(RFC3339),For receiving end records.
	SentAt string
}

type webhookItem struct {
	FindingID     int64
	Name          string
	VulnClass     string
	Severity      string
	SeverityLabel string
	Summary       string
	Assets        []string
	DetailURL     string
	FromStatus    string
	ToStatus      string
	// StatusLabel is a readable description of a change of status, if[Pending → Fixed];Empty when non-state change.
	StatusLabel string
}

func (webhookChannel) Validate(cfg map[string]any) error {
	raw := cfgString(cfg, "url")
	if raw == "" {
		return errors.New("Missing target URL")
	}
	if err := validateHTTPURL(raw); err != nil {
		return fmt.Errorf("Target URL Invalid: %w", err)
	}
	if m := strings.ToUpper(cfgString(cfg, "method")); m != "" && m != http.MethodGet && m != http.MethodPost && m != http.MethodPut && m != http.MethodPatch {
		return fmt.Errorf("Unsupported Method %s(Available GET/POST/PUT/PATCH)", m)
	}
	if tpl := cfgString(cfg, "body_template"); tpl != "" {
		if _, err := parseWebhookTemplate(tpl); err != nil {
			return fmt.Errorf("Request body template syntax error: %w", err)
		}
	}
	return nil
}

func (c webhookChannel) Send(ctx context.Context, cfg map[string]any, m Message) (int, error) {
	if err := c.Validate(cfg); err != nil {
		return 0, Permanent(err)
	}
	method := strings.ToUpper(cfgString(cfg, "method"))
	if method == "" {
		method = http.MethodPost
	}

	// GET Unrequested: Plug in query It's beyond the capabilities of the template, and it's not in line. GET Semantic,
	// So GET It only fits.[Hit or trigger hook.]This recipients.
	var payload any
	if method != http.MethodGet {
		body, err := renderWebhookBody(cfgString(cfg, "body_template"), m)
		if err != nil {
			return 0, Permanent(err)
		}
		// Template is rendered in string form JSON,Here. json.RawMessage Send as Is,
		// Avoid secondary transposition to fit user-built structures JSON String.
		if !json.Valid([]byte(body)) {
			return 0, Permanent(errors.New("The requested template rendering is not valid JSON"))
		}
		payload = json.RawMessage(body)
	}

	headers := cfgMap(cfg, "headers")
	if ct := cfgString(cfg, "content_type"); ct != "" {
		// Allows overlay, but places headers Then apply and ensure that visible configuration takes precedence.
		if headers == nil {
			headers = map[string]string{}
		}
		headers["Content-Type"] = ct
	}
	if _, err := doJSON(ctx, method, cfgString(cfg, "url"), headers, payload); err != nil {
		return 0, err
	}
	// General Webhook Do not cut the body (the receiving end is the user's own service, by volume) body_template Decision),
	// That's why the whole batch was delivered..
	return len(m.Items), nil
}

// renderWebhookBody Render requests with user templates (or default templates) Body.
func renderWebhookBody(tpl string, m Message) (string, error) {
	if strings.TrimSpace(tpl) == "" {
		tpl = webhookDefaultTemplate
	}
	t, err := parseWebhookTemplate(tpl)
	if err != nil {
		return "", fmt.Errorf("Request body template syntax error: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, newWebhookTemplateData(m)); err != nil {
		return "", fmt.Errorf("Rendering request template failed: %w", err)
	}
	return buf.String(), nil
}

// parseWebhookTemplate Parsing Template.
//
// missingkey=zero Let the missing map Key render to zero instead of reporting errors——But the context of this document is structural.,
// The main role is Jean. .Items is empty range No mistake. What really needs to be protected. .Items for nil.
func parseWebhookTemplate(tpl string) (*template.Template, error) {
	return template.New("body").Funcs(webhookTemplateFuncs).Option("missingkey=zero").Parse(tpl)
}

// webhookTemplateFuncs It's an auxiliary function exposed to the template..
var webhookTemplateFuncs = template.FuncMap{
	// json Sequence Any Value into JSON.
	//
	// It's not about adding flowers, it's about what's necessary: to save it, the user can only write. {{.Title}} Direct Plugin Value,
	// And as long as there are quotation marks or line breaks in the headline, the entire body is no longer valid. JSON——Receiver meeting
	// Refusal and misdirection.[JSON Parsing failed],I can't believe there's a quote in the title..
	"json": func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		return string(raw), nil
	},
	// jsons Used to JSON Snippet embedding another part JSON Intra-string value (do a string conversion)).
	"jsons": func(v any) (string, error) {
		raw, err := json.Marshal(v)
		if err != nil {
			return "", err
		}
		quoted, err := json.Marshal(string(raw))
		if err != nil {
			return "", err
		}
		// Remove the outer quote: the caller decides whether to add a quote..
		return string(quoted[1 : len(quoted)-1]), nil
	},
}

func newWebhookTemplateData(m Message) webhookTemplateData {
	d := webhookTemplateData{
		Title:   markdownTitle(m),
		Batch:   m.Batch,
		Count:   len(m.Items),
		HomeURL: m.HomeURL,
		SentAt:  time.Now().Format(time.RFC3339),
		Items:   make([]webhookItem, 0, len(m.Items)),
	}
	for _, it := range m.Items {
		wi := webhookItem{
			FindingID:     it.FindingID,
			Name:          it.Name,
			VulnClass:     it.VulnClass,
			Severity:      it.Severity,
			SeverityLabel: SeverityLabel(it.Severity),
			Summary:       it.Summary,
			Assets:        append([]string{}, it.Assets...),
			DetailURL:     it.DetailURL,
			FromStatus:    it.FromStatus,
			ToStatus:      it.ToStatus,
		}
		if it.IsStatusChange() {
			wi.StatusLabel = StatusLabel(it.FromStatus) + " → " + StatusLabel(it.ToStatus)
		}
		d.Items = append(d.Items, wi)
	}
	return d
}
