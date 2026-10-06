package notify

import (
	"context"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// This document locks Universal Webhook Template**Capacity boundaries**.
//
// It's the only one.[The string provided by the user is used as a code for value]The place, so make it clear what it can do.,
// We can't do anything, and we'll fix it with tests.——Otherwise, someone's going to add one to the template.
// Method, or FuncMap Riga. readFile,The power level has expanded. diff Looks like...
// It's just a harmless little function..

// TestTemplateContextHasNoMethods It's the most important one..
//
// text/template You can call the export method.({{.Foo}} You can both take fields and adjust methods. So the template context
// As long as you can reach it.**Any**The type that takes out the methods is the same as exposing them to the author of the template..
// The context of this function is deliberately pure data (export fields only, zero methods)).
//
// If this fails: webhookTemplateData / webhookItem I added the method..
// Before you decide to let it go, figure out if that method can be used to read things you don't want to expose..
func TestTemplateContextHasNoMethods(t *testing.T) {
	for _, v := range []any{webhookTemplateData{}, webhookItem{}} {
		typ := reflect.TypeOf(v)
		if n := typ.NumMethod(); n != 0 {
			var names []string
			for i := 0; i < n; i++ {
				names = append(names, typ.Method(i).Name)
			}
			t.Fatalf("%s It's exposed. %d Method(%s):text/template We can call them.,"+
				"It's like opening up these methods to the author.", typ.Name(), n, strings.Join(names, ", "))
		}
	}
}

// TestTemplateFuncsAreMinimal Lock a set of functions exposed to templates.
//
// FuncMap Each of these functions has an additional ability. Currently only json / jsons,The effect is to sequence values.
// Done. JSON fragment——No documents, no requests, no orders..
func TestTemplateFuncsAreMinimal(t *testing.T) {
	var got []string
	for name := range webhookTemplateFuncs {
		got = append(got, name)
	}
	sort.Strings(got)
	want := []string{"json", "jsons"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Template functions have changed: Got %v,Expectations %v.Make sure it doesn't expand before adding a function Noodles."+
			"(Unable to read and write files, not to launch network requests, not to execute orders)", got, want)
	}
}

// TestTemplateCannotReachUnknownData Overwrite cross-border access in the template:
// Access to what does not exist must fail, not highlight something; and failure information must not take out internal data.
func TestTemplateCannotReachUnknownData(t *testing.T) {
	_, err := renderWebhookBody(`{"x": {{.Environment}}, "y": {{.Env}}}`, singleMsg())
	if err == nil {
		t.Fatal("Error should be reported when accessing non-existent fields")
	}
	// Could not close temporary folder: %s/Abstract).
	for _, leak := range []string{"SQLInjection", "Parameter id"} {
		if strings.Contains(err.Error(), leak) {
			t.Errorf("Template error leaked message %q: %v", leak, err)
		}
	}
}

// TestTemplateRenderFailsPermanently Template error is a configuration error and retrying does not heal itself.
// If you're convicted of retrying, a bad template will allow every delivery to run three times in vain..
func TestTemplateRenderFailsPermanently(t *testing.T) {
	cfg := map[string]any{
		"url":           "https://example.com/hook",
		"body_template": `{{.Items.`,
	}
	if err := (webhookChannel{}).Validate(cfg); err == nil {
		t.Fatal("Template syntax error should be stopped while saving")
	}
	// Even if you bypass the verification direct delivery, you have to judge permanent failure rather than repeated attempts..
	_, err := (webhookChannel{}).Send(context.Background(), cfg, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("Bad template should be permanently defeated. Got it. %v", err)
	}
}

// TestTemplateCanOnlyProduceJSON override[Template rendering must be valid JSON]This is a constraint..
// It's blocked.[Generate plain text with templates to trigger other protocols]Use of this kind.
func TestTemplateCanOnlyProduceJSON(t *testing.T) {
	// Legitimate templates pass.
	ok := map[string]any{"url": "https://example.com/hook", "body_template": `{"t":{{json .Title}}}`}
	if err := (webhookChannel{}).Validate(ok); err != nil {
		t.Fatalf("Legitimate templates should be verified: %v", err)
	}
	// Rendering Africa JSON It has to be rejected (not sent as it is).).
	bad := map[string]any{"url": "http://127.0.0.1:1/hook", "body_template": `not json {{.Count}}`}
	_, err := (webhookChannel{}).Send(context.Background(), bad, singleMsg())
	if err == nil || !IsPermanent(err) {
		t.Fatalf("Rendering Africa JSON You're going to lose forever. %v", err)
	}
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Errorf("The wrong message should say yes. JSON Problem. Got it. %v", err)
	}
}
