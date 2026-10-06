package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// Other Organiser doJSON of HTTP Layer error rating.
//
// Why do you have to do it alone? errcode,
// Feishu code,Telegram ok fields), and**HTTP Layer**The grade is... doJSON One thing.,
// They are two separate lines of defence. One way back. 503 The transit gateway will be considered a permanent failure.,
// Directly abandon retry; one 403 They'll try again and leave three rounds for nothing..

func replyServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDoJSONClassifiesHTTPStatus(t *testing.T) {
	cases := []struct {
		name      string
		status    int
		permanent bool
	}{
		{"200 Success is not a mistake.", 200, false},
		{"429 Limit to try again", 429, false},
		{"408 Request timeout to try again", 408, false},
		{"500 Service error retryable", 500, false},
		{"502 Gateway error retryable", 502, false},
		{"503 Service not available for retrying", 503, false},
		{"400 Parameter error permanently failed", 400, true},
		{"401 It's a permanent failure.", 401, true},
		{"403 Deny access permanently failed", 403, true},
		{"404 Chile", 404, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := replyServer(t, tc.status, `{"detail":"upstream says no"}`)
			_, err := doJSON(context.Background(), "GET", srv.URL, nil, nil)
			if tc.status < 300 {
				if err != nil {
					t.Fatalf("2xx No mistakes.: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("Not 2xx Wrong-doing.")
			}
			if got := IsPermanent(err); got != tc.permanent {
				t.Fatalf("HTTP %d of permanent Error of determination: expectation %v get %v (%v)",
					tc.status, tc.permanent, got, err)
			}
			// The status code has to appear in the error, otherwise the user can't judge whether it's wrong or wrong..
			// The numbers, not the numbers. Go English StatusText:This package is in Chinese.
			// (In line with the rest of the project) figures are the non-linguistic, stable assertion..
			if !strings.Contains(err.Error(), strconv.Itoa(tc.status)) {
				t.Errorf("The wrong message should be on. HTTP Status code %d,get %v", tc.status, err)
			}
		})
	}
}

// TestDoJSONIncludesResponseSnippet override snippet:The error note for the back end is to be brought back.,
// Otherwise the user only knows[It failed.],I don't know why they refused..
func TestDoJSONIncludesResponseSnippet(t *testing.T) {
	srv := replyServer(t, 400, `{"error":"invalid webhook token"}`)
	_, err := doJSON(context.Background(), "GET", srv.URL, nil, nil)
	if err == nil {
		t.Fatal("Wrong-doing.")
	}
	if !strings.Contains(err.Error(), "invalid webhook token") {
		t.Errorf("The error message should be returned to the end of the line. %v", err)
	}
}

// TestDoJSONSnippetIsSingleLineAndBounded Constraints snippet Form:
// It's going to respond to the end as it is. last_error Columns and front-end tables, multiple rows/It's too long to destroy the layout and load..
func TestDoJSONSnippetIsSingleLineAndBounded(t *testing.T) {
	// Bring line breaks, tabs and 5000 Responsiveness to the excessive content of characters.
	long := strings.Repeat("x", 5000)
	srv := replyServer(t, 500, "line1\nline2\r\n\tline3 "+long)
	_, err := doJSON(context.Background(), "GET", srv.URL, nil, nil)
	if err == nil {
		t.Fatal("Wrong-doing.")
	}
	msg := err.Error()
	if strings.ContainsAny(msg, "\r\n\t") {
		t.Errorf("The error message should be pressed in a single line. %q", msg)
	}
	// snippet upper limit 200 Characters + Fixed prefix. The total must be much smaller than the original sound. Response.
	if len(msg) > 400 {
		t.Errorf("Error message too long(%d bytes), should be snippet Cut: %q", len(msg), msg)
	}
}

// TestDoJSONRejectsOversizedResponse Confirm reading cap: End-to-end anomaly returns super-maximum Time
// You can't read the entire response into the memory. last_error).
func TestDoJSONRejectsOversizedResponse(t *testing.T) {
	huge := strings.Repeat("A", 1<<20) // 1 MiB
	srv := replyServer(t, 400, huge)
	_, err := doJSON(context.Background(), "GET", srv.URL, nil, nil)
	if err == nil {
		t.Fatal("Wrong-doing.")
	}
	if len(err.Error()) > 400 {
		t.Errorf("The super-heavy response should be read and cut long, and the error message length %d", len(err.Error()))
	}
}
