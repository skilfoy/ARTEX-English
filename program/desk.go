package program

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Desk is the per-task record: engagement graph, run log, fireteam, and score.
// It is stored as one JSON file. It is not the exploration graph and it does
// not replace cold-graph compaction.
type Desk struct {
	mu     sync.Mutex
	dir    string
	Graph  Graph
	Log    []Event
	Team   Fireteam
	Record *Run
	Lease  Lease
}

// Graph is the engagement graph. Kinds are fixed so a label cannot smuggle
// a procedure in through the type field.
type Graph struct {
	Nodes []GNode `json:"nodes"`
	Edges []GEdge `json:"edges"`
}

// GNode is one engagement object.
type GNode struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
	Ref   string `json:"ref,omitempty"`
}

// GEdge links two nodes.
type GEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// Event is one line of the run log. Secret-shaped fields are dropped.
type Event struct {
	At     time.Time         `json:"at"`
	Kind   string            `json:"kind"`
	Fields map[string]string `json:"fields,omitempty"`
}

// Fireteam is a set of specialist roles sharing one desk.
// Strict refuses a finding write until Confirm. The default is advisory:
// a seated team is recorded and does not block writes.
type Fireteam struct {
	Members   []Member `json:"members"`
	Collected bool     `json:"collected"`
	Confirmed bool     `json:"confirmed"`
	Strict    bool     `json:"strict,omitempty"`
}

// Member is one specialist. Note is a status, not a procedure.
type Member struct {
	Role   string `json:"role"`
	Note   string `json:"note,omitempty"`
	Status string `json:"status"`
}

// Run is the score of one harness pass. It records coverage, not a method.
type Run struct {
	Harness      string  `json:"harness"`
	Revision     string  `json:"revision"`
	Model        string  `json:"model"`
	ScopeHash    string  `json:"scopeHash"`
	Coverage     float64 `json:"coverage"`
	Precision    float64 `json:"precision"`
	Depth        int     `json:"depth"`
	ROEFailures  int     `json:"roeFailures"`
	Noise        int     `json:"noise"`
	Reproducible bool    `json:"reproducible"`
}

var nodeKinds = map[string]bool{
	"asset": true, "identity": true, "service": true,
	"finding": true, "evidence": true, "scope-group": true, "plane": true,
}

var roles = map[string]bool{
	"recon": true, "identity": true, "web": true, "evidence": true, "retest": true,
}

type deskFile struct {
	Graph  Graph    `json:"graph"`
	Log    []Event  `json:"log"`
	Team   Fireteam `json:"team"`
	Record *Run     `json:"record,omitempty"`
	Lease  Lease    `json:"lease,omitempty"`
}

// Lease is a short hold so two operators do not confirm the same desk.
// An empty or expired lease blocks nobody.
type Lease struct {
	Owner string    `json:"owner,omitempty"`
	Until time.Time `json:"until,omitempty"`
}

// Open loads a desk directory, creating it if needed.
func Open(dir string) (*Desk, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	desk := &Desk{dir: dir}
	raw, err := os.ReadFile(desk.path())
	if err != nil {
		if os.IsNotExist(err) {
			return desk, nil
		}
		return nil, err
	}
	var file deskFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, err
	}
	desk.Graph = file.Graph
	desk.Log = file.Log
	desk.Team = file.Team
	desk.Record = file.Record
	desk.Lease = file.Lease
	return desk, nil
}

func (d *Desk) path() string { return filepath.Join(d.dir, "desk.json") }

// Save writes the desk atomically.
func (d *Desk) Save() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	raw, err := json.MarshalIndent(deskFile{Graph: d.Graph, Log: d.Log, Team: d.Team, Record: d.Record, Lease: d.Lease}, "", "  ")
	if err != nil {
		return err
	}
	tmp := d.path() + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, d.path())
}

// AddNode inserts a node. Duplicate ids are rejected.
func (d *Desk) AddNode(id, kind, label, ref string) error {
	if !nodeKinds[kind] {
		return fmt.Errorf("desk: unknown node kind %q", kind)
	}
	if id == "" || strings.ContainsAny(id, " \t\r\n") {
		return fmt.Errorf("desk: bad node id")
	}
	if ok, reason := ScanText(label + "\n" + ref); !ok {
		return fmt.Errorf("desk: %s", reason)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, node := range d.Graph.Nodes {
		if node.ID == id {
			return fmt.Errorf("desk: duplicate node %s", id)
		}
	}
	d.Graph.Nodes = append(d.Graph.Nodes, GNode{ID: id, Kind: kind, Label: label, Ref: ref})
	return nil
}

// Link connects two existing nodes.
func (d *Desk) Link(from, to, kind string) error {
	if kind == "" {
		return fmt.Errorf("desk: empty edge kind")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.has(from) || !d.has(to) {
		return fmt.Errorf("desk: edge endpoints must exist")
	}
	d.Graph.Edges = append(d.Graph.Edges, GEdge{From: from, To: to, Kind: kind})
	return nil
}

func (d *Desk) has(id string) bool {
	for _, node := range d.Graph.Nodes {
		if node.ID == id {
			return true
		}
	}
	return false
}

// Append records a log event. Fields whose names look like secrets are dropped.
func (d *Desk) Append(kind string, fields map[string]string) {
	clean := map[string]string{}
	for key, value := range fields {
		if secretField(key) || secretValue(value) {
			continue
		}
		clean[key] = value
	}
	d.mu.Lock()
	d.Log = append(d.Log, Event{At: time.Now().UTC(), Kind: kind, Fields: clean})
	if len(d.Log) > 2000 {
		d.Log = d.Log[len(d.Log)-2000:]
	}
	d.mu.Unlock()
}

func secretField(key string) bool {
	key = strings.ToLower(key)
	for _, word := range []string{"password", "secret", "token", "authorization", "vault", "cookie"} {
		if strings.Contains(key, word) {
			return true
		}
	}
	return false
}

func secretValue(value string) bool {
	if strings.Contains(value, "AKIA") || strings.Contains(value, "BEGIN PRIVATE KEY") || strings.Contains(value, "BEGIN OPENSSH PRIVATE KEY") {
		return true
	}
	return false
}

// AddRole adds a specialist. Confirm is cleared because the team changed.
func (d *Desk) AddRole(role, note string) error {
	if !roles[role] {
		return fmt.Errorf("desk: unknown role %q", role)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, member := range d.Team.Members {
		if member.Role == role {
			return fmt.Errorf("desk: role %s already seated", role)
		}
	}
	d.Team.Members = append(d.Team.Members, Member{Role: role, Note: note, Status: "open"})
	d.Team.Collected = false
	d.Team.Confirmed = false
	return nil
}

// MarkDone marks one role finished.
func (d *Desk) MarkDone(role string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	for i := range d.Team.Members {
		if d.Team.Members[i].Role == role {
			d.Team.Members[i].Status = "done"
			d.Team.Collected = false
			d.Team.Confirmed = false
			return nil
		}
	}
	return fmt.Errorf("desk: role %s is not seated", role)
}

// Collect succeeds only when every seated role is done.
func (d *Desk) Collect() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.Team.Members) == 0 {
		return fmt.Errorf("desk: no specialists seated")
	}
	for _, member := range d.Team.Members {
		if member.Status != "done" {
			return fmt.Errorf("desk: %s is not done", member.Role)
		}
	}
	d.Team.Collected = true
	return nil
}

// Confirm allows finding writes. Collect must have succeeded.
func (d *Desk) Confirm() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.Team.Collected {
		return fmt.Errorf("desk: collect the fireteam before confirming")
	}
	d.Team.Confirmed = true
	return nil
}

// SetStrict turns the write gate on or off. Turning it on clears confirm
// so a previously advisory team cannot write until it is collected again.
func (d *Desk) SetStrict(on bool) {
	d.mu.Lock()
	d.Team.Strict = on
	if on {
		d.Team.Collected = false
		d.Team.Confirmed = false
	}
	d.mu.Unlock()
}

// SeatAdvisory seats any missing roles. Errors for an unknown or already
// seated role are ignored so a second worker start is a no-op. Strict is
// left untouched.
func (d *Desk) SeatAdvisory(roles []string) {
	for _, role := range roles {
		_ = d.AddRole(role, "advisory")
	}
}

// MayWrite reports whether a finding may be recorded.
// An empty team is not a fireteam, so writes stay allowed.
// A seated team blocks only when Strict is set and Confirm has not run.
func (d *Desk) MayWrite() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.Team.Strict || len(d.Team.Members) == 0 {
		return true
	}
	return d.Team.Confirmed
}

// Acquire holds the desk for owner until ttl elapses. A second owner is refused
// while the hold is unexpired.
func (d *Desk) Acquire(owner string, ttl time.Duration) error {
	if strings.TrimSpace(owner) == "" || ttl <= 0 {
		return fmt.Errorf("desk: bad lease")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Lease.Owner != "" && d.Lease.Owner != owner && time.Now().Before(d.Lease.Until) {
		return fmt.Errorf("desk: leased to %s", d.Lease.Owner)
	}
	d.Lease.Owner = owner
	d.Lease.Until = time.Now().Add(ttl)
	return nil
}

// Allows reports whether owner may confirm. An empty or expired lease allows
// anyone. A live lease allows only its owner.
func (d *Desk) Allows(owner string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Lease.Owner == "" || !time.Now().Before(d.Lease.Until) {
		return true
	}
	return owner != "" && owner == d.Lease.Owner
}

// Recent returns the last n log events. This is the executor view: the log,
// not a generated story.
func (d *Desk) Recent(n int) []Event {
	d.mu.Lock()
	defer d.mu.Unlock()
	if n > len(d.Log) {
		n = len(d.Log)
	}
	if n <= 0 {
		return nil
	}
	out := make([]Event, n)
	copy(out, d.Log[len(d.Log)-n:])
	return out
}

// RecordFixture stores a score for a scripted walk. It is not a harness
// result and it does not contact a network.
func (d *Desk) RecordFixture(scopeHash string) error {
	return d.SetRun(Run{
		Harness:      "fixture",
		Revision:     "catalog",
		Model:        "none",
		ScopeHash:    scopeHash,
		Coverage:     0,
		Precision:    1,
		Depth:        0,
		Reproducible: true,
	})
}

// SetRun stores a score after checking its shape.
func (d *Desk) SetRun(run Run) error {
	if err := run.Valid(); err != nil {
		return err
	}
	d.mu.Lock()
	d.Record = &run
	d.mu.Unlock()
	return nil
}

// Valid checks the score bounds. The scope hash is sha256 hex.
func (r Run) Valid() error {
	if strings.TrimSpace(r.Harness) == "" {
		return fmt.Errorf("run: harness is required")
	}
	if len(r.ScopeHash) != 64 {
		return fmt.Errorf("run: scope hash must be 64 hex characters")
	}
	if _, err := hex.DecodeString(r.ScopeHash); err != nil {
		return fmt.Errorf("run: scope hash is not hex")
	}
	if r.Coverage < 0 || r.Coverage > 1 || r.Precision < 0 || r.Precision > 1 {
		return fmt.Errorf("run: coverage and precision must be between 0 and 1")
	}
	if r.Depth < 0 || r.ROEFailures < 0 || r.Noise < 0 {
		return fmt.Errorf("run: counts must not be negative")
	}
	return nil
}

// HashText is a stand-in scope hash for tests and for callers that have the
// scope bytes but not a precomputed digest.
func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
