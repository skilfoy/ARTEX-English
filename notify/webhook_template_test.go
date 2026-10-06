package notify

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This file locks the capability boundary of the generic webhook template.
//
// It is the only place in the package where a user-supplied string is
// evaluated as code, so what it can and cannot do has to be explicit and
// pinned by tests. Otherwise someone later adds a method to the template
// context, or a readFile helper to FuncMap, and the capability surface grows
// silently while the diff looks like a harmless little function.

// TestTemplateContextHasNoMethods is the important one.
//
// text/template calls exported methods ({{.Foo}} reads a field or calls a
// method). If the template context can reach any type with exported methods,
// those methods are exposed to the template author. This feature's context
// is deliberately pure data: exported fields only, zero methods.
//
// If this fails, someone added a method to webhookTemplateData or
// webhookItem. Before allowing that, decide whether the template can use
// the method to read something that should not be exposed.
func TestTemplateContextHasNoMethods(t *testing.T) {
	for _, v := range []any{webhookTemplateData{}, webhookItem{}} {
		typ := reflect.TypeOf(v)
		if n := typ.NumMethod(); n != 0 {
			var names []string
			for i := 0; i < n; i++ {
				names = append(names, typ.Method(i).Name)
			}
			t.Fatalf("%s exposes %d method(s) (%s): text/template can call them, "+
				"which hands those capabilities to the template author", typ.Name(), n, strings.Join(names, ", "))
		}
	}
}

// TestTemplateFuncsAreMinimal locks the set of functions exposed to templates.
//
// Every extra FuncMap entry is an extra capability. Today there are only
// json and jsons, and all they do is serialize a value into a JSON fragment.
// They cannot read files, send requests, or run commands.
func TestTemplateFuncsAreMinimal(t *testing.T) {
	var got []string
	for name := range webhookTemplateFuncs {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"json", "jsons"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("template functions changed: got %v, want %v. Before adding a function, "+
			"confirm it does not widen the capability surface "+
			"(no file access, no network requests, no command execution)", got, want)
	}
}

// TestTemplateCannotReachUnknownData covers out-of-bounds access in a template:
// reaching something that does not exist must fail, not echo a value, and the
// failure must not include internal data.
func TestTemplateCannotReachUnknownData(t *testing.T) {
	_, err := renderWebhookBody(`{"x": {{.Environment}}, "y": {{.Env}}}`, singleMsg())
	if err == nil {
		t.Fatal("accessing a missing field should fail")
	}
	// The error must not contain real content from the template context
	// (finding title or summary).
	for _, leak := range []string{"SQLInjection", "Parameter id"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("template error leaked message content %q: %v", leak, err)
		}
	}
}

// TestTemplateRenderFailsPermanently: a bad template is a config error and
// retrying will not fix it. If it were classified as retryable, one bad
// template would burn three backoff rounds on every delivery.
func TestTemplateRenderFailsPermanently(t *testing.T) {
	cfg := map[string]any{
		"url":           "https://example.com/hook",
		"body_template": `{{.Items.`,
	}
	if err := (webhookChannel{}).Validate(cfg); err == nil {
		t.Fatal("a template syntax error should be rejected at save time")
	}
	// Even if validation is bypassed and Send is called directly, it must be
	// a permanent failure rather than a retry.
	_, err := (webhookChannel{}).Send(context.Background(), cfg, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("a bad template should be a permanent failure, got %v", err)
	}
}

// TestTemplateCanOnlyProduceJSON covers the rule that a rendered template
// must be valid JSON. It also blocks using the template to emit plain text
// that triggers some other protocol.
func TestTemplateCanOnlyProduceJSON(t *testing.T) {
	// A valid template is accepted.
	ok := map[string]any{"url": "https://example.com/hook", "body_template": `{"t":{{json .Title}}}`}
	if err := (webhookChannel{}).Validate(ok); err != nil {
		t.Fatalf("a valid template should pass validation: %v", err)
	}
	// A render that is not JSON must be rejected, not sent as-is.
	bad := map[string]any{"url": "http://127.0.0.1:1/hook", "body_template": `not json {{.Count}}`}
	_, err := (webhookChannel{}).Send(context.Background(), bad, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("rendering non-JSON should be a permanent failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Errorf("the error should say this is a JSON problem, got %v", err)
	}
}
