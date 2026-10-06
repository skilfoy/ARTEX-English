package notify

// Snapshot is the contract for the notification_events.snapshot JSONB column.
// The db layer writes it inside the finding-insert transaction; the server
// layer's delivery engine reads it for filtering. It lives in this package
// because it is the payload of the notification domain: db only serializes
// it and does not interpret the fields.
//
// Finding fields are copied here instead of looked up at render time because
// a finding can later be renamed, re-graded, or have its status changed.
// The notification must reflect the conclusion at the time of the event.
// Looking it up again would show a finding that was later downgraded to low,
// which is a dangerous misread. Fan-out and rendering also avoid joining
// findings, tasks, and assets.
type Snapshot struct {
	// Event kind: finding_created or finding_status_changed.
	Kind      string  `json:"kind"`
	FindingID int64   `json:"finding_id"`
	TaskID    int64   `json:"task_id"`
	VulnClass string  `json:"vulnclass"`
	Name      string  `json:"name"`
	Severity  string  `json:"severity"`
	Summary   string  `json:"summary"`
	AssetIDs  []int64 `json:"asset_ids"`
	// Set only when kind is finding_status_changed.
	FromStatus string `json:"from_status,omitempty"`
	ToStatus   string `json:"to_status,omitempty"`
}

// Item is one finding waiting to be rendered by a channel.
type Item struct {
	FindingID int64
	Name      string
	VulnClass string
	Severity  string
	Summary   string
	// Assets holds resolved display names (a domain or IP, for example).
	// The server layer fills them in. This package does not touch the
	// database, so it cannot look the names up itself.
	Assets []string
	// DetailURL links back to the finding. Empty means public_base_url is
	// not configured, and renderers omit the link.
	DetailURL string
	// Used only for status-change events. When both are set, renderers show
	// them as "Pending → Fixed".
	FromStatus string
	ToStatus   string
}

// IsStatusChange reports whether the item is a status-change event.
func (i Item) IsStatusChange() bool { return i.FromStatus != "" || i.ToStatus != "" }

// Title returns the display title: the human-given name, then the vulnclass,
// then a placeholder. It never returns an empty title.
func (i Item) Title() string {
	if i.Name != "" {
		return i.Name
	}
	if i.VulnClass != "" {
		return i.VulnClass
	}
	return "(unnamed finding)"
}

// Message is everything one channel send needs.
type Message struct {
	// Length 1 for a single push; a whole batch for a digest.
	// An empty slice is invalid. The caller must supply at least one item.
	Items []Item
	// Batch selects digest rendering: a different title, plus the time window
	// and the count.
	Batch bool
	// WindowMinutes is the digest period, in minutes. It is used only when
	// Batch is true, for the "past N minutes" wording. It is passed in from
	// config rather than computed with time.Since at render time so rendering
	// stays deterministic and testable.
	WindowMinutes int
	// HomeURL is the console address (the global public_base_url). Empty
	// means the message has no console link.
	HomeURL string
}
