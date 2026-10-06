package notify

// Snapshot Yes notification_events.snapshot This one. JSONB Lines of contract. It's written by db Layer
// It's a leaking library. server Layers of delivery engines match filters. The definition is in this bag because it is.
// [Areas of notification]The payload.:db Just sequencing. I don't understand what a field means..
//
// Why are redundant gaps fields not checked back when they are being rendered: they are subsequently renamed, graded, state changed,
// And the thrust should reflect**When it happened,**Conclusions——We'll get it back.[It was changed to low]of
// Dangerously misleading. And... fan-out It doesn't have to be rendered. JOIN findings/tasks/assets Three tables..
type Snapshot struct {
	// Event type:finding_created / finding_status_changed
	Kind      string  `json:"kind"`
	FindingID int64   `json:"finding_id"`
	TaskID    int64   `json:"task_id"`
	VulnClass string  `json:"vulnclass"`
	Name      string  `json:"name"`
	Severity  string  `json:"severity"`
	Summary   string  `json:"summary"`
	AssetIDs  []int64 `json:"asset_ids"`
	// Only kind=finding_status_changed Time is not empty.
	FromStatus string `json:"from_status,omitempty"`
	ToStatus   string `json:"to_status,omitempty"`
}

// Item It's a loophole to be sent. It's for the channel..
type Item struct {
	FindingID int64
	Name      string
	VulnClass string
	Severity  string
	Summary   string
	// Assets It's a deconstructed asset display (e.g. domain name)/IP).By server Layer Fill——
	// This bag doesn't touch the database. I can't get a name..
	Assets []string
	// DetailURL is the loophole detail return chain; empty not matched public_base_url,omission while rendering.
	DetailURL string
	// Change of status event specific; both are not space-time rendering Done.[Pending → Fixed].
	FromStatus string
	ToStatus   string
}

// IsStatusChange Report whether the entry is a status change event.
func (i Item) IsStatusChange() bool { return i.FromStatus != "" || i.ToStatus != "" }

// Title Return entry display title: Priority for manual naming name,Type of loophole returned vulnclass,
// Both with a placeholder.——Never output empty title.
func (i Item) Title() string {
	if i.Name != "" {
		return i.Name
	}
	if i.VulnClass != "" {
		return i.VulnClass
	}
	return "(Unnamed Hole)"
}

// Message It's a complete source..
type Message struct {
	// The length of the single slide is 1;Summary transfer(digest)It's a whole batch..
	// Empty slices are illegal. Callers must guarantee at least one..
	Items []Item
	// Batch=true Rendering by Summary Message (change title, time window, number of bars)).
	Batch bool
	// WindowMinutes Summary cycle (minutes), only Batch=true Time used for writing[near N min].
	// When deliberately transposed rather than retrofitted, Fine. time.Since:Rendering remains certain..
	WindowMinutes int
	// HomeURL is the platform panel address (global) public_base_url);Empty without panel entrance.
	HomeURL string
}
