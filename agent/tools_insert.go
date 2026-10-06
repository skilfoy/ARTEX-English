package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	actool "github.com/Autumn-27/norma/tool"
	"github.com/skilfoy/ARTEX-English/db"
)

// assetInterceptCandidates Extract a domain name to insert into the asset entry/IP/URL Candidate string for asset intercept matching.
// URL of host ♪ will tear out the categories and make ♪[Only URL]Services/End-point assets can also be used by domain names/IP Rules hit..
func assetInterceptCandidates(item assetInputItem) (domains, ips, urls []string) {
	add := func(dst *[]string, s string) {
		if s = strings.TrimSpace(s); s != "" {
			*dst = append(*dst, s)
		}
	}
	add(&domains, item.Domain)
	for _, d := range item.BoundDomains {
		add(&domains, d)
	}
	add(&ips, item.IP)
	add(&ips, item.ServiceIP)
	add(&urls, item.URL)
	if item.URL != "" {
		if u, err := url.Parse(item.URL); err == nil {
			if h := u.Hostname(); h != "" {
				if net.ParseIP(h) != nil {
					add(&ips, h)
				} else {
					add(&domains, h)
				}
			}
		}
	}
	return domains, ips, urls
}

// assetInputLabel Return a short identifier of the asset to be inserted to intercept the description.
func assetInputLabel(item assetInputItem) string {
	typ := strings.TrimSpace(item.Type)
	var target string
	switch {
	case strings.TrimSpace(item.Domain) != "":
		target = strings.TrimSpace(item.Domain)
	case strings.TrimSpace(item.URL) != "":
		target = strings.TrimSpace(item.URL)
	case strings.TrimSpace(item.IP) != "":
		target = strings.TrimSpace(item.IP)
	case strings.TrimSpace(item.ServiceIP) != "":
		target = strings.TrimSpace(item.ServiceIP)
	default:
		target = "(Unknown)"
	}
	if typ != "" {
		return fmt.Sprintf("[%s] %s", typ, target)
	}
	return target
}

// =====================================================================
// Unified asset insertion tools
// =====================================================================

// SetAssetStore wires the asset store and company store onto this ToolSet
// so the insert_assets, add_company_scope, and list_assets tools are active.
func (t *ToolSet) SetAssetStore(as *db.AssetStore, cs *db.CompanyStore) {
	t.as = as
	t.cs = cs
}

// assetInputItem is one element of the insert_assets "assets" array.
type assetInputItem struct {
	Type string `json:"type"` // root_domain|ip|subdomain|app|service|endpoint

	// ---- root_domain / subdomain ----
	Domain      string   `json:"domain"`
	ICP         string   `json:"icp"`
	RecordType  string   `json:"record_type"`
	RecordValue []string `json:"record_value"`

	// ---- ip ----
	IP           string           `json:"ip"`
	BoundDomains []string         `json:"bound_domains"`
	OpenPorts    []db.PortService `json:"open_ports"`

	// ---- app ----
	AppName     string `json:"app_name"`
	BundleID    string `json:"bundle_id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	AppICP      string `json:"app_icp"`
	CompanyID   *int64 `json:"company_id"` // explicit company link (app only; others auto-attribute via scope)

	// ---- service (http) ----
	URL           string           `json:"url"`
	Technologies  []string         `json:"technologies"`
	StatusCode    *int             `json:"status_code"`
	ContentLength *int64           `json:"content_length"`
	PageTitle     string           `json:"page_title"`
	FaviconMMH3   string           `json:"favicon_mmh3"`
	Auth          []map[string]any `json:"auth"`
	ServiceName   string           `json:"service_name"`
	ServiceIP     string           `json:"service_ip"` // optional enrichment IP

	// ---- service (other) ----
	Port  int    `json:"port"`
	Proto string `json:"proto"`

	// ---- endpoint ----
	Method string           `json:"method"`
	Params []map[string]any `json:"params"`
}

// insertAssets is the unified insert_assets agent tool.
func (t *ToolSet) insertAssets() actool.CoreTool {
	return writeTool(
		"insert_assets",
		"Batch registration of newly discovered assets, which can be mixed in multiple types at a time(type See enumeration).\n"+
			"Fields for All Types:root_domain→domain;ip→ip(shall be IPv4/IPv6,Non-host name);subdomain→domain;app→app_name;service(HTTP)→url;service(NotHTTP)→service_name+port(ip/domain At least one.);endpoint→url+method.For the rest of the fields, see the respective notes..\n"+
			"auth/technologies/params To Add Merge(append),Do not overwrite original value.\n"+
			"Return:{results:[{index,id,type}], errors:[{index,error}]}",
		obj(map[string]any{
			// task_id Not exposed to models:worker Which one? task By process SetTaskID Authority(See handler).
			"assets": map[string]any{
				"type":        "array",
				"description": "Asset arrays, one asset record for each element",
				"items": obj(map[string]any{
					"type": map[string]any{
						"type":        "string",
						"enum":        []string{"root_domain", "ip", "subdomain", "app", "service", "endpoint"},
						"description": "Asset type",
					},
					// root_domain / subdomain
					"domain":      str("Root domain name or subdomain name(root_domain/subdomain Required)"),
					"icp":         str("ICP Record number (optional))"),
					"record_type": str("DNS Analysis type:A/AAAA/CNAME/MX etc.(subdomain Optional)"),
					"record_value": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "DNS Parsing Value List(subdomain Optional [\"1.2.3.4\",\"2.3.4.5\"])",
					},
					// ip
					"ip": str("IP Address, must be IPv4/IPv6 Address, cannot fill hostname (hostname requested) type=subdomain of domain Field);ip Type to fill;service/endpoint Type to fill in for association IP"),
					"bound_domains": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "The IP List of bound domain names(ip Type Optional)",
					},
					"open_ports": map[string]any{
						"type":        "array",
						"description": "Open Port List(ip Type Optional)",
						"items": obj(map[string]any{
							"port":    intp("Port Number"),
							"service": str("Name of service, e.g. http/ssh/mysql Wait (optional))"),
						}, "port"),
					},
					// app
					"app_name":    str("Application name(app Type to fill)"),
					"bundle_id":   str("Bundle ID(app Type Optional)"),
					"category":    str("Apply classification (optional))"),
					"description": str("Apply description (optional))"),
					"app_icp":     str("Application ICP Recording (optional))"),
					"company_id":  intp("Official enterprise id(app Type Optional;app I can't. scope Auto attribution with visible designation.id By add_company_scope Return)"),
					// service (http)
					"url":         str("Complete URL,Include protocols and ports(HTTP Service must be filled.;service_type Set As http)"),
					"status_code": intp("HTTP Response status code, for example 200/301/403/404(Optional)"),
					"content_length": map[string]any{
						"type":        "integer",
						"description": "HTTP Response bytes (optional))",
					},
					"page_title":   str("Page <title> Contents (optional)"),
					"favicon_mmh3": str("favicon MMH3 Hash.)"),
					"technologies": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Fingerprint/List of technology stacks, if [\"Nginx\",\"Vue\",\"Bootstrap\"](Optional)",
					},
					"auth": map[string]any{
						"type":        "array",
						"description": "List of authentication information found, each containing type/username/password Parameters (optional, additional not covered))",
						"items":       map[string]any{"type": "object"},
					},
					// service (other,Not HTTP)
					"service_name": str("Name of service, e.g. ssh/mysql/redis(service Not HTTP Always.)"),
					"port":         intp("Port Number(service Not HTTP Always.)"),
					// endpoint
					"method": str("HTTP Method:GET/POST/PUT/PATCH/DELETE etc.(endpoint Required)"),
					"params": map[string]any{
						"type":        "array",
						"description": "Request list of parameters, each containing location(query/body/header/path)/name/value/type(Optional, Add Uncovered)",
						"items":       map[string]any{"type": "object"},
					},
				}, "type"),
			},
		}, "assets"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("insert_assets Not enabled: AssetStore Not initialized"), nil
			}
			var a struct {
				Assets []assetInputItem `json:"assets"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf("invalid input: " + err.Error()), nil
			}
			// task_id Valued by program authority(worker: SetTaskID),No model entry accepted——Avoid Model Leaking/Error
			// As a result, assets were not returned to or misdirected. Caller without task context(auto/pentest/chat)Other t.taskID=0.
			taskID := t.taskID

			type result struct {
				Index int    `json:"index"`
				ID    int64  `json:"id"`
				Type  string `json:"type"`
			}
			type errEntry struct {
				Index int    `json:"index"`
				Error string `json:"error"`
			}

			var results []result
			var errs []errEntry

			// Asset gate rules are entered once; reading failure skips the decision (without blocking insertion)).
			// Interception rules = Global ∪ Task level block;Allow rules = Task level allow.
			blockRules, _ := t.as.ListAssetInterceptRules()
			var allowRules []db.AssetInterceptRule
			if t.taskID > 0 {
				if tb, ta, err := t.as.TaskInterceptRulesSplit(t.taskID); err == nil {
					blockRules = append(blockRules, tb...)
					allowRules = ta
				}
			}

			for i, item := range a.Assets {
				// Asset gates: Interception before permission is granted and rejected assets are prohibited from insertion (jumping) Upsert and subsequent side effects).
				domains, ips, urls := assetInterceptCandidates(item)
				if d := db.EvaluateAssetGate(blockRules, allowRules, domains, ips, urls); !d.Allowed {
					errs = append(errs, errEntry{
						Index: i,
						Error: fmt.Sprintf("Assets %s %s,Inserting is prohibited", assetInputLabel(item), d.Reason),
					})
					continue
				}

				typ := strings.TrimSpace(item.Type)
				var id int64
				var err error

				switch typ {
				case "root_domain":
					id, err = t.as.UpsertRootDomain(db.UpsertRootDomainReq{
						Domain: item.Domain,
						ICP:    item.ICP,
						TaskID: taskID,
					})

				case "ip":
					id, err = t.as.UpsertIP(db.UpsertIPReq{
						IP:           item.IP,
						BoundDomains: item.BoundDomains,
						OpenPorts:    item.OpenPorts,
						TaskID:       taskID,
					})

				case "subdomain":
					id, err = t.as.UpsertSubdomain(db.UpsertSubdomainReq{
						Domain:      item.Domain,
						RecordType:  item.RecordType,
						RecordValue: item.RecordValue,
						ICP:         item.ICP,
						TaskID:      taskID,
					})

				case "app":
					id, err = t.as.UpsertApp(db.UpsertAppReq{
						Name:        item.AppName,
						BundleID:    item.BundleID,
						Category:    item.Category,
						Description: item.Description,
						ICP:         item.AppICP,
						CompanyID:   item.CompanyID,
						TaskID:      taskID,
					})

				case "service":
					// distinguish HTTP vs other by presence of url
					if item.URL != "" {
						// agent may send "ip" or "service_ip" for the enrichment IP; accept both
						svcIP := item.ServiceIP
						if svcIP == "" {
							svcIP = item.IP
						}
						id, err = t.as.UpsertHTTPService(db.UpsertHTTPServiceReq{
							URL:           item.URL,
							Technologies:  item.Technologies,
							StatusCode:    item.StatusCode,
							ContentLength: item.ContentLength,
							PageTitle:     item.PageTitle,
							FaviconMMH3:   item.FaviconMMH3,
							Auth:          item.Auth,
							IP:            svcIP,
							TaskID:        taskID,
						})
					} else {
						id, err = t.as.UpsertOtherService(db.UpsertOtherServiceReq{
							Domain:      item.Domain,
							IP:          item.IP,
							Port:        item.Port,
							ServiceName: item.ServiceName,
							Auth:        item.Auth,
							TaskID:      taskID,
						})
					}

				case "endpoint":
					id, err = t.as.UpsertEndpoint(db.UpsertEndpointReq{
						URL:    item.URL,
						Method: item.Method,
						Params: item.Params,
						IP:     item.ServiceIP,
						TaskID: taskID,
					})

				default:
					errs = append(errs, errEntry{Index: i, Error: "unknown type: " + typ})
					continue
				}

				if err != nil {
					errs = append(errs, errEntry{Index: i, Error: err.Error()})
					continue
				}
				results = append(results, result{Index: i, ID: id, Type: typ})
				t.writes.Assets++
				t.anchorOwner(id)
				if taskID > 0 {
					var sourceNodeID *int64
					if t.ownerNode > 0 {
						nodeID := t.ownerNode
						sourceNodeID = &nodeID
					}
					summary := "Agent Pass insert_assets Register"
					if t.ownerNode > 0 {
						summary = fmt.Sprintf("Worker Intention #%d Pass insert_assets Register", t.ownerNode)
					}
					_ = t.as.SetTaskAssetSource(taskID, id, "agent", summary, sourceNodeID)
				}
				// Auto-test range(source='auto'):Only worker This top-level visible insertion, press this
				// Type with Conservative Scope;side-effect The derived assets are not here, so they are not blindly expanded..taskID=0 Timeless.
				// It's not about covering switches.:task_scope It's the mission boundary.(list/Filter benchmarks for queries),
				// The overlay switch only determines whether to use it as a denominator to calculate the indicator, not whether to accumulate the range itself..
				{
					svcIP := item.ServiceIP
					if svcIP == "" {
						svcIP = item.IP
					}
					_ = t.as.AddAutoScope(taskID, typ, item.Domain, item.URL, svcIP)
				}
			}

			return jsonResult(map[string]any{
				"results": results,
				"errors":  errs,
			})
		},
	)
}

// addCompanyScope writes to company_scope table and triggers asset attribution.
func (t *ToolSet) addCompanyScope() actool.CoreTool {
	return writeTool(
		"add_company_scope",
		"Organisation/IP/CIDR/ICPRecord/Enterprise keyword to join a company[Asset scope]——Domain name, network andICPAssets will be automatically claimed, and keywords will only be providedAgentAs a range hint.\n"+
			"Company names are unique. Create a new company when needed, or add scope to an existing company.\n"+
			"scope One line, automatic system recognition: root domain name / URL / Single IP / CIDR Network segment / ICPRecord / Enterprise keyword.\n"+
			"It must be. reason Description of the basis of attribution(whois/Certificate/ASN etc.).\n"+
			"Guard: Refuse nudity TLD With a wide band(IPv4Prefix to/16-/32,IPv6Prefix to/32-/128),Illegal practices can be bypassed and errors Return.",
		obj(map[string]any{
			"company": str("Unique company name"),
			"scope":   str("Scope of assets, one row: domain name / URL / IP / CIDR / ICPRecord / Enterprise keyword"),
			"reason":  str("Basis of attribution(Evidence/source),Always fill"),
			"logo":    str("Company Icon URL(optional; effective on start-up only)"),
		}, "company", "scope"),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.cs == nil {
				return actool.Errorf("add_company_scope Not enabled: CompanyStore Not initialized"), nil
			}
			var a struct {
				Company string `json:"company"`
				Scope   string `json:"scope"`
				Reason  string `json:"reason"`
				Logo    string `json:"logo"`
			}
			if err := json.Unmarshal(in, &a); err != nil {
				return actool.Errorf(err.Error()), nil
			}
			if strings.TrimSpace(a.Company) == "" {
				return actool.Errorf("company Cannot be empty"), nil
			}
			companyID, _, err := t.cs.UpsertCompany(a.Company, a.Logo)
			if err != nil {
				return actool.Errorf("Create/Failed to get company: " + err.Error()), nil
			}
			lines := splitLines(a.Scope)
			added, skipped, invalid, errMsgs := t.cs.AddScope(companyID, lines, a.Reason)
			out := map[string]any{
				"company_id": companyID,
				"added":      added,
				"skipped":    skipped,
				"invalid":    invalid,
			}
			if len(errMsgs) > 0 {
				out["errors"] = errMsgs
			}
			return jsonResult(out)
		},
	)
}

// addTaskScope lets the plan agent add test scope to THE CURRENT TASK — the coverage
// denominator and the task's authorization edge. Worker discoveries are auto-scoped
// (precise host) by insertAssets; this tool is for DELIBERATELY WIDENING: pull a whole
// root domain or whole company into scope, or add a specific subdomain / ip.
func (t *ToolSet) addTaskScope() actool.CoreTool {
	return writeTool(
		"add_task_scope",
		"Add the test range.[This task]——This is the authorized boundary of this mission and the denominator of the asset-test coverage..\n"+
			"kind Support:company(The entire company's assets.) / root_domain(Entire root domain, including all sub-areas) / subdomain(Single Accuracy Sublevel) / ip / cidr / icp / keyword.\n"+
			"Instructions:worker One-on-one, the main opportunity is being systematically encountered.[Automatic]Add Range(Accurate subarea);This tool is used for[Proactive expansion]——Put the whole root field/Entire company included or additionally assigned a sub-area/IP.\n"+
			"value:company Call the company name or id(The company has to exist.);root_domain/subdomain Domain Name;ip/cidr Pass IP Or a segment;icp/keyword File number or business keyword.\n"+
			"It must be. reason Statement of basis(Auditable).Multiple entries array.",
		obj(map[string]any{
			"entries": map[string]any{"type": "array", "description": "Batch:[{kind, value}].kind∈company/root_domain/subdomain/ip/cidr/icp/keyword.", "items": map[string]any{"type": "object"}},
			"kind":    str("[Single] company / root_domain / subdomain / ip / cidr / icp / keyword"),
			"value":   str("[Single] Company name orid / Domain name / IP / CIDR / ICP / Keywords"),
			"reason":  str("Basis for accession(For audit),Always fill"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("add_task_scope Not enabled: AssetStore Not initialized"), nil
			}
			if t.taskID <= 0 {
				return actool.Errorf("add_task_scope Task context required(Current None task)"), nil
			}
			type scopeEntry struct {
				Kind  string `json:"kind"`
				Value string `json:"value"`
			}
			var a struct {
				Entries    []scopeEntry `json:"entries"`
				scopeEntry              // Single Bar Mode
				Reason     string       `json:"reason"`
			}
			_ = json.Unmarshal(in, &a)
			items := a.Entries
			if len(items) == 0 {
				items = []scopeEntry{a.scopeEntry}
			}
			var added []map[string]any
			errs := map[string]string{}
			for i, e := range items {
				ts, err := t.as.AddAgentScope(t.taskID, strings.TrimSpace(e.Kind), e.Value, a.Reason, "agent")
				if err != nil {
					errs[strconv.Itoa(i)] = err.Error()
					continue
				}
				added = append(added, map[string]any{"kind": ts.Kind, "domain": ts.Domain, "net": ts.Net, "value": ts.Value, "company_id": ts.CompanyID})
			}
			out := map[string]any{"added": added}
			if len(errs) > 0 {
				out["errors"] = errs
			}
			return jsonResult(out)
		},
	)
}

// listUntestedAssets lets the plan agent pull the current + directly inherited
// scope's not-yet-tested assets on demand (filter by type, paginated).
func (t *ToolSet) listUntestedAssets() actool.CoreTool {
	return readTool(
		"list_untested_assets",
		"Query[This mandate and its direct link]Assets within the range, not covered by the de facto anchor.).\n"+
			"Optional Filter by Asset Type:root_domain/subdomain/service/app/endpoint/ip.\n"+
			"Page:page from 1 from,page_size Default 10.Return {assets:[{id,type,label}], total, page, page_size}.Task context only available.",
		obj(map[string]any{
			"type":      str("Asset type filter (optional)):root_domain/subdomain/service/app/endpoint/ip"),
			"page":      intp("Page number from 1 Start (default) 1)"),
			"page_size": intp("Number per page (default) 10)"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("list_untested_assets Not enabled: AssetStore Not initialized"), nil
			}
			if t.taskID <= 0 || t.ts == nil {
				return actool.Errorf("list_untested_assets Task context required"), nil
			}
			var a struct {
				Type     string `json:"type"`
				Page     int    `json:"page"`
				PageSize int    `json:"page_size"`
			}
			_ = json.Unmarshal(in, &a)
			if a.Page <= 0 {
				a.Page = 1
			}
			if a.PageSize <= 0 {
				a.PageSize = 10
			}
			offset := (a.Page - 1) * a.PageSize
			assets, total, err := t.as.ListUntestedAssetsWithSources(t.taskID, strings.TrimSpace(a.Type), a.PageSize, offset)
			if err != nil {
				return actool.Errorf(err.Error()), nil
			}
			return jsonResult(map[string]any{
				"assets": assets, "total": total, "page": a.Page, "page_size": a.PageSize,
			})
		},
	)
}

// listAssets lets an agent query the asset table.
func (t *ToolSet) listAssets() actool.CoreTool {
	return readTool(
		"list_assets",
		"Search assets within this task and its direct scope by DSL expression or asset ID. Results are paginated.\n"+
			"DSL operators: field=value for a case-insensitive partial match, field==value for an exact match, and field!=value for exclusion. Numeric fields support >, >=, <, and <=. Bare words search across fields. Combine expressions with AND, OR, and parentheses; AND has higher precedence. Use the separate type parameter to filter asset types.\n"+
			"Provide dsl, id, or ids. Unfiltered queries are not allowed.\n"+
			"Fields: domain, root_domain, ip, url, page_title, icp, service_name, app_name, method, service_type, record_type, technology, port, status_code, and company_id.\n"+
			"Examples: status_code>=400 AND technology=shiro; (port==80 OR port==443) AND technology=nginx",
		obj(map[string]any{
			"dsl":    str("Asset search expression. Required when id and ids are absent."),
			"type":   str("Optional asset type: root_domain, ip, subdomain, app, service, or endpoint."),
			"id":     intp("Optional ID of one asset."),
			"ids":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Optional IDs of several assets."},
			"limit":  intp("Maximum number of results. Default: 10."),
			"offset": intp("Number of results to skip. Default: 0."),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.as == nil {
				return actool.Errorf("list_assets is unavailable because the asset store is not initialized"), nil
			}
			var a struct {
				DSL    string  `json:"dsl"`
				Type   string  `json:"type"`
				ID     int64   `json:"id"`
				IDs    []int64 `json:"ids"`
				Limit  int     `json:"limit"`
				Offset int     `json:"offset"`
			}
			_ = json.Unmarshal(in, &a)
			if a.Limit <= 0 {
				a.Limit = 10
			}

			var assets []*db.Asset
			var err error
			switch {
			case a.ID > 0:
				assets, err = t.as.GetByIDsInScope(t.taskID, []int64{a.ID})
			case len(a.IDs) > 0:
				assets, err = t.as.GetByIDsInScope(t.taskID, a.IDs)
			case a.DSL != "":
				assets, err = t.as.QueryDSLInScope(a.DSL, a.Type, t.taskID, a.Limit, a.Offset)
			default:
				return actool.Errorf("Unsigned id/ids hour dsl Unable to empty: Unconditional search of all assets is not allowed, please provide conditions for searching"), nil
			}
			if err != nil {
				return actool.Errorf("DSL Error: " + err.Error()), nil
			}
			return jsonResult(map[string]any{
				"count":  len(assets),
				"assets": assets,
			})
		},
	)
}

// listCompanies lets an agent enumerate companies (Enterprise) with their scope + asset count.
func (t *ToolSet) listCompanies() actool.CoreTool {
	return readTool(
		"list_companies",
		"Listing of assets in the inventory[Enterprise/Company]and scope of its assets(scope)Compared to the number of assets attributed.,"+
			"Get it company_id(insert_assets Association app,list_assets Press company_id Use while filtering)."+
			"Optional search Filter by company name(Case sensitive),Free Return All.",
		obj(map[string]any{
			"search": str("Filter by company name(Optional, Caseless);Free Return All"),
		}),
		func(_ context.Context, in json.RawMessage) (actool.Result, error) {
			if t.cs == nil {
				return actool.Errorf("list_companies Not enabled: CompanyStore Not initialized"), nil
			}
			var a struct {
				Search string `json:"search"`
			}
			_ = json.Unmarshal(in, &a)
			cos, err := t.cs.ListCompanies()
			if err != nil {
				return actool.Errorf("Failed to query company: " + err.Error()), nil
			}
			q := strings.ToLower(strings.TrimSpace(a.Search))
			type companyOut struct {
				ID         int64    `json:"id"`
				Name       string   `json:"name"`
				AssetCount int      `json:"asset_count"`
				Scope      []string `json:"scope"`
			}
			out := make([]companyOut, 0, len(cos))
			for _, c := range cos {
				if q != "" && !strings.Contains(strings.ToLower(c.Name), q) {
					continue
				}
				scope := make([]string, 0, len(c.Scope))
				for _, r := range c.Scope {
					scope = append(scope, r.Raw)
				}
				out = append(out, companyOut{ID: c.ID, Name: c.Name, AssetCount: c.AssetCount, Scope: scope})
			}
			return jsonResult(map[string]any{"count": len(out), "companies": out})
		},
	)
}

// splitLines splits a multi-line string into non-empty trimmed lines.
func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// WorkerTools returns the tool set for a work agent.
func (t *ToolSet) WorkerTools() []actool.CoreTool {
	return []actool.CoreTool{
		// list_findings Reservations: Check this mandate before reporting any gaps and avoid reporting the same..
		t.listFindings(),
		t.addFinding(), t.recordFact(),
		// asset management (handlers guard nil store internally).
		// add_company_scope I won't. worker:Defining the scope of enterprise assets as planning/Master/Auto Duties,worker Only explore..
		t.insertAssets(), t.listAssets(),
		// Cross work Look back.:worker It can be reused. work Watch and avoid duplication of effort.
		// search_all_worker_traces:I don't need to know. intent_id,Global hit by keyword;
		// get_worker_trace:Lock something work Next steps/Search everywhere./Take Full Contents.
		t.searchAllWorkerTraces(), t.getWorkerTrace(),
		// node_detail:worker Get it intent_id/node id Then you can check the full details of the node. Look.).
		t.nodeDetail(),
		// The following tools remain[I won't.]worker,I'll leave it to you. planner/main(Read context, cross work Rewinding is a planning function.,
		// worker Only execution and writing back of a single intent.):list_facts / list_companies / list_worker_traces.
	}
}

// MainAgentTools returns the human-interface tool set.
func (t *ToolSet) MainAgentTools() []actool.CoreTool {
	return []actool.CoreTool{
		t.graphOverview(), t.listFindings(), t.listFacts(), t.nodeDetail(),
		t.expandDigest(), // cold-digest §6.1
		t.getWorkerOutput(), t.getWorkerTrace(), t.searchAllWorkerTraces(), t.addHint(), t.addIntent(),
		// steer_work:A person can be expected to run an article(work)Real time injection of correction instructions (no interruption, no loss of progress)).
		t.steerWorkTool(),
		// set_goals:A person can add a new ultimate objective to the mission at the time of its operation.).
		t.setGoals(),
		// set_constraints:You can supplement this task while running/Change Operating Constraints(allow/deny),Constraints planner/worker The Exploration of Borders.
		t.setConstraints(),
		// asset management (handlers guard nil store internally)
		t.insertAssets(), t.addCompanyScope(), t.listAssets(),
		t.addFinding(), t.recordFact(),
		t.addTaskScope(),
		// list_untested_assets:Unscheduled assets under this mandate(Type+Page),Make up your mind..
		t.listUntestedAssets(),
	}
}

// AllDomainTools returns the union of all domain tools across all agent types,
// deduped by name (mainagent order wins). Used by the server to build a registry
// for injecting domain tools into agents (Auto, custom) that don't own a per-task
// ToolSet. The caller provides real stores; tools are callable at taskID=0 scope.
func (t *ToolSet) AllDomainTools() []actool.CoreTool {
	seen := map[string]bool{}
	var out []actool.CoreTool
	all := append(append(t.MainAgentTools(), t.PlannerTools()...), t.WorkerTools()...)
	for _, tool := range all {
		if !seen[tool.Name()] {
			seen[tool.Name()] = true
			out = append(out, tool)
		}
	}
	return out
}
