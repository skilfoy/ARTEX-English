package selfupdate

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Repo Is the release source. Write to die instead of as a configuration item: Update the source equals one for anyone who can reconfigure
// It's a remote code execution channel. It's not open for a penetration test platform..
const Repo = "skilfoy/ARTEX-English"

// latestURL Yes GitHub of"Updated official version"Interface. It skips automatically. prerelease and draft.
const latestURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// allowedHosts Defines the domain name that the upgrade link can access. Get down there. checkRedirect,
// Any host whose jump is redirected to outside the list will simply fail.——It's a precaution. DNS Pollution / Intermediaries
// The first gate to replace the binary, the second is... SHA256SUMS Match.
var allowedHosts = map[string]bool{
	"api.github.com":                       true,
	"github.com":                           true,
	"objects.githubusercontent.com":        true, // release Physically landed object storage of assets
	"release-assets.githubusercontent.com": true,
	"raw.githubusercontent.com":            true,
}

// Release Yes GitHub Release The fields we care about.
type Release struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	HTMLURL     string    `json:"html_url"`
	Assets      []Asset   `json:"assets"`
}

// Asset Yes Release A file on top.
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// NewClient Construct a recognition only GitHub Domain Name HTTP Client.proxy It's empty..
//
// Do not use default again deliberately Transport:The upgrade must be forced. TLS And you can't be anywhere else.
// Settings InsecureSkipVerify And so on..
func NewClient(proxy string) *http.Client {
	tr := &http.Transport{
		ForceAttemptHTTP2:   true,
		TLSHandshakeTimeout: 15 * time.Second,
	}
	if p := strings.TrimSpace(proxy); p != "" {
		if pu, err := url.Parse(p); err == nil {
			tr.Proxy = http.ProxyURL(pu)
		}
	}
	return &http.Client{
		Transport: tr,
		Timeout:   30 * time.Minute, // Download the whole package. You can't die as requested.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("Too many redirections")
			}
			return checkURL(req.URL)
		},
	}
}

// checkURL Force https + White list of domain names.
func checkURL(u *url.URL) error {
	if u.Scheme != "https" {
		return fmt.Errorf("Refuse HTTPS Address: %s", u.Scheme+"://"+u.Host)
	}
	if !allowedHosts[strings.ToLower(u.Hostname())] {
		return fmt.Errorf("Refuse GitHub Domain name: %s", u.Hostname())
	}
	return nil
}

// FetchLatest Query an updated official version.
func FetchLatest(ctx context.Context, c *http.Client) (*Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return nil, err
	}
	if err := checkURL(req.URL); err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "artex-selfupdate")

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Visits GitHub Failed (can configure global agents in system settings)): %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusTooManyRequests:
		// Uncertified GitHub API It's every IP Hourly 60 Second, common export IP It's easy to hit..
		return nil, fmt.Errorf("GitHub Interface restricted flow (per hour) 60 Please try again later.")
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("Warehouse %s No official version has yet been published", Repo)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("GitHub Return %d", resp.StatusCode)
	}

	var rel Release
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("Analysis Release Failed: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return nil, fmt.Errorf("Release Missing tag")
	}
	return &rel, nil
}

// AssetName Returns the release name of the current platform, and build.sh of package_binary Consistency:
// artex-<version>-<os>-<arch>.zip(No version number v Prefix).
func AssetName(tag, goos, goarch string) string {
	return fmt.Sprintf("artex-%s-%s-%s.zip", strings.TrimPrefix(tag, "v"), goos, goarch)
}

// FindAsset at Release Lee looks for assets by name..
func (r *Release) FindAsset(name string) (Asset, bool) {
	for _, a := range r.Assets {
		if strings.EqualFold(a.Name, name) {
			return a, true
		}
	}
	return Asset{}, false
}
