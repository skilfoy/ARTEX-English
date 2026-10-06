package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
)

// allowLocalTargets Whether or not to allow the message to be sent back to the ring / Local address for links.
//
// Default rejection. These are not the addresses. IM Where robots or public network mail servers will appear, and they can.
// It's sensitive: a management port for another service, and metadata for the cloud environment. Points
// (169.254.169.254,An example is available. The delivery address was matched by the administrator, but one was... XSS/CSRF
// A management session borrowed or shared JWT The second person can read the response back by changing the configuration.
// ——doJSON will 4xx/5xx Other Organiser 200 Byte Write last_error,And deliver history interfaces
// That's a half-blind reading..
//
// But...[Here. SMTP Relay](127.0.0.1:25 Top postfix)It's a common configuration for custom mail.,
// One cut will hold people. That's why we left a visible escape without a hard code. Okay.:
// Settings ARTEX_NOTIFY_ALLOW_LOCAL=1 Allow.
//
// Export As AllowLocalTargetsEnv It's to get the test to open it clearly.——This package and server The bag.
// Extensive use of examples 127.0.0.1 Top httptest If you don't open it, the guards will stop you..
const AllowLocalTargetsEnv = "ARTEX_NOTIFY_ALLOW_LOCAL"

func allowLocalTargets() bool {
	v := strings.TrimSpace(os.Getenv(AllowLocalTargetsEnv))
	return v == "1" || strings.EqualFold(v, "true")
}

// isBlockedDialIP Reporting objectives IP Whether it belongs to[Default not allowed delivery]Chile.
//
// Only reject ring return, link local (with cloud metadata) 169.254.169.254),No program specified.
// **Not Rejected** RFC1918 Private networking: built-in intranet Mattermost / SMTP Relay is a common legal usage.,
// If you block them together, the function is simply not available in the real environment. It's a trade-off.——
// Protect against truly sensitive targets, while not eliminating normal deployments..
func isBlockedDialIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	// IPv4-mapped IPv6(::ffff:127.0.0.1)To restore it. IPv4 Let's do it again, or we'll go around..
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

// blockInternalDial Yes http.Transport Dialer. Control The hook.**When connection is created**
// Check destination address.
//
// Why is it at the dialup stage instead of only checking while saving configuration: this is the final effective point?.
// It covers both scenarios that bypass the configuration check.——DNS Reset (resolve to public network during validation) IP,
// When you're actually connected, resolve to the inner net) and redirect (although we have refused to jump across the mainframe, we jump with the host)
// It's still possible to point the path elsewhere.).
func blockInternalDial(_, address string, _ syscall.RawConn) error {
	if allowLocalTargets() {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("Cannot parse target address %q", host)
	}
	if isBlockedDialIP(ip) {
		return fmt.Errorf("Refuse delivery to this machine/Local address for links %s(If you really need to deliver to this service, settings %s=1)", ip, AllowLocalTargetsEnv)
	}
	return nil
}

// notifyTransport On Default Transport Only one dial-up guard is added to the base..
// Use Clone Keep all the adjustments of the default (connecting pool),HTTP/2,Timeout,proxy etc.),
// Avoid changing other behaviors for one additional inspection.
var notifyTransport = func() *http.Transport {
	t, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{}
	}
	clone := t.Clone()
	clone.DialContext = (&net.Dialer{Timeout: 10 * time.Second, Control: blockInternalDial}).DialContext
	return clone
}()

// httpClient It's a common client for all channels..
//
// I mean it.**No**Global Export Agent for Reused Projects(server Side. GlobalProxy):That agent is for infiltration.
// Target traffic is often used in unstable tunnels, and the availability of notification should not be kidnapped by the target network's vibrations..
// IM Just push straight to the company. Timeout As 15 second——Slower than this is actually a malfunction..
//
// Deny cross-host redirection: this function is both sent to[One fixed endpoint]Form. Normal.
// Redirect to another host; these are the proofs. access_token,Micro key,Telegram of
// bot token)**Right here. URL inside**,Following a cross-host jump equals giving the proof to a re-direction target. Same Host
// Jump (e.g. end slash) is still allowed.
var httpClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: notifyTransport,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("Too many redirections")
		}
		if len(via) > 0 && req.URL.Host != via[0].URL.Host {
			return fmt.Errorf("Deny cross-host reorientation(%s → %s)", via[0].URL.Host, req.URL.Host)
		}
		return nil
	},
}

// respBodyLimit Limits the size of the reader response. You know, it's like we're gonna throw it back when the end is abnormal.
// The error code and a small error description are used to show it in the delivery history.
const respBodyLimit = 8 << 10

// doJSON Send one request and return response (limited) Long).
//
// payload for nil Organisation body(for GET Or not required by the platform body The scene).
// headers in which the value is attached as it is, for general use Webhook Custom Header.
//
// Error classification is the core function of this function: network failure and 5xx/408/429 Attribution[Retry],
// The rest 4xx Attribution[Permanent Failure]——Try again 403 Just brushing the same mistake. 3 Through Logs.
func doJSON(ctx context.Context, method, url string, headers map[string]string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			// Serialization failure is local bug(Configure field types not correct).
			return nil, Permanent(fmt.Errorf("Construct requested body failed: %w", err))
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		// URL Illegal——Most of the users filled out the wrong address..
		// There's no such thing as passing through here. err:url.Parse The error text contains the full address.
		return nil, Permanent(fmt.Errorf("Requesting address is illegal: %s", redactRequestTarget(url)))
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		// Connection denied,DNS Failure, timeout.——It's mostly instantaneous failure..
		//
		// Error text must be unsensitized before being passed on. Reason:http.Client.Do Return *url.Error,
		// It's... Error() Yes `Op "CompleteURL": Bottom Error`,And it's the evidence of these people.**Right here. URL inside**
		// (DingTalk access_token,Micro key,Feishu hook id,Telegram /bot<token>/).
		// If you're not allergic, the evidence will follow this error to four places.:notification_deliveries
		// of last_error(Reactions to express library, delivery of historical interfaces(**Surround the channel configuration mask**),
		// Service-end logs, and test sending interface back to frontend 502 Text.
		return nil, fmt.Errorf("Request Failed: %s", redactTransportError(err))
	}
	defer resp.Body.Close()
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, respBodyLimit))
	if readErr != nil {
		return nil, fmt.Errorf("Failed to read response: %w", readErr)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return raw, nil
	}
	// 429() and 408(Timeout) is worth trying again; the rest 4xx It's a question of configuration or permission. It's pointless to try again..
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusRequestTimeout {
		return nil, fmt.Errorf("Competing time limit or overtime (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	if resp.StatusCode >= 500 {
		return nil, fmt.Errorf("The client's service is abnormal. (HTTP %d): %s", resp.StatusCode, snippet(raw))
	}
	return nil, Permanent(fmt.Errorf("The other side denied the request. (HTTP %d): %s", resp.StatusCode, snippet(raw)))
}

// snippet Presss the response body into a line of short text for error information. There may be a change of line and a lot of gaps in the response,
// Right in. last_error It's gonna blow up the slide page..
func snippet(raw []byte) string {
	return OneLine(string(raw), 200)
}

// redactRequestTarget Press the delivery address.[scheme://host/…],For Error Information.
//
// It's the only address in this bag that's allergic.**That's rough enough.**:Except... scheme With host,
// The rest is abandoned. The reason is not one.[Universal and safe.]How to judge URL Which part is based on evidence?:
//
//	Nailed. query      /robot/send?access_token=xxx
//	The evidence. query      /cgi-bin/webhook/send?key=xxx
//	Flying Book.**End of Path** /open-apis/bot/v2/hook/<hook_id>
//	Telegram On the evidence.**Center segment of the path** /bot<token>/sendMessage
//
// Yes.[Keep only the useful part]You're gonna have to go through the patch, and the missing family is a leak..
// Reservations host It's enough for the search.(DNS I can't figure it out. I can't get it. I can't get it right.),
// Specifically, which robot is identified by a coded tail signal from the channel configuration..
//
// Return fixed placeholder when parsing failed——Never reveal the original string..
func redactRequestTarget(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "(Address Not Parsed)"
	}
	return u.Scheme + "://" + u.Host + "/…"
}

// redactTransportError Remove the address from the error of the transfer layer and keep only the bottom reason.
//
// *url.Error The structure is... {Op, URL, Err},Error() will URL Let's fight it..
// Here you go. Err Fields, bypass it. Error() ——More reliable than replacing strings later,
// Because the replacement has to be right. URL Encode/The mutations of transposition are easy to miss..
func redactTransportError(err error) string {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		host := ""
		if u, parseErr := url.Parse(uerr.URL); parseErr == nil {
			host = u.Host
		}
		if uerr.Err != nil {
			return fmt.Sprintf("%s %s: %s", uerr.Op, host, uerr.Err)
		}
		return fmt.Sprintf("%s %s: Unknown error", uerr.Op, host)
	}
	// Not *url.Error(It's also possible to take an address and get dissensitized..
	return redactURLsInText(err.Error())
}

// redactURLsInText Puts a text in it. http(s) Address replaced with dissensitisation form.
//
// To pry out errors that do not capture structured fields (redirective strategy errors, custom errors in third-party libraries)).
// Only recognized http/https Prefix, split by blank and quotation marks——Address does not contain these two characters.
func redactURLsInText(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		rest := s[i:]
		if strings.HasPrefix(rest, "http://") || strings.HasPrefix(rest, "https://") {
			end := len(rest)
			if j := strings.IndexAny(rest, " \t\n\"'"); j >= 0 {
				end = j
			}
			b.WriteString(redactRequestTarget(rest[:end]))
			i += end
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
