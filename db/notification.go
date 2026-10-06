package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/skilfoy/ARTEX-English/notify"
)

// What is this document? IM Send channel configuration and event layer. The recipient and status of the delivery task are now in the current.
// db/notification_delivery.go.
//
// Two variables. Make sure this file is maintained.:
//
//  1. Writing for bugs(RecordFindingTx)Call Only InsertNotificationEventTx One blind plug.,
//     Do not read any notice-related tables, do not filter matches. Any reading introduced here could be due to
//     The user has wrong filter conditions and contaminated or even aborted the bugs..
//  2. Filter Match Never Wrong: Configure malformations always press[hit]Processing(See notify.Match).I'd rather push.,
//     Don't let it slip..

// ErrNotificationChannelNotFound There is no channel..
var ErrNotificationChannelNotFound = errors.New("No channels of notification exist")

// Organisation.
const (
	NotifyStatePending = "pending" // To be issued
	NotifyStateSending = "sending" // It's been one of them. dispatcher Received, lease not due
	NotifyStateSent    = "sent"    // Delivered
	NotifyStateFailed  = "failed"  // A re-test is exhausted or permanently failed and can re-activate manually
	NotifyStateSkipped = "skipped" // Channel disabled, not sent
)

// Send Mode.
const (
	NotifyModeRealtime = "realtime"
	NotifyModeDigest   = "digest"
)

// ValidNotifyMode White List Verify Send Mode (with findings.status Same: no. DB CHECK,
// To facilitate subsequent expansion).
func ValidNotifyMode(m string) bool {
	return m == NotifyModeRealtime || m == NotifyModeDigest
}

// NotificationChannel A channel case configuration.Config With Filter Keep original JSON,
// Give it to me. notify Package——db I don't understand what their fields mean..
type NotificationChannel struct {
	ID     int64           `json:"id"`
	Name   string          `json:"name"`
	Kind   string          `json:"kind"`
	Mode   string          `json:"mode"`
	Config json.RawMessage `json:"config"`
	Filter json.RawMessage `json:"filter"`
	// Enabled The pointer is to distinguish.[No message.]With[Visibility false]——
	// Front-end switch control only submits modified fields.
	Enabled    *bool     `json:"enabled,omitempty"`
	RatePerMin int       `json:"rate_per_min"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// IsEnabled Return channel enabled;Enabled for nil(Unmounted) by Enable.
func (c *NotificationChannel) IsEnabled() bool { return c.Enabled == nil || *c.Enabled }

// NotificationEvent It's a fact..
type NotificationEvent struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	FindingID int64           `json:"finding_id"`
	Snapshot  json.RawMessage `json:"snapshot"`
	CreatedAt time.Time       `json:"created_at"`
}

const notificationChannelCols = `id, name, kind, enabled, config, mode, filter, rate_per_min, created_at, updated_at`

func scanNotificationChannel(sc interface{ Scan(...any) error }) (*NotificationChannel, error) {
	var c NotificationChannel
	var enabled bool
	if err := sc.Scan(&c.ID, &c.Name, &c.Kind, &enabled, &c.Config, &c.Mode, &c.Filter, &c.RatePerMin, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return nil, err
	}
	c.Enabled = &enabled
	return &c, nil
}

// ListNotificationChannels Returns the example of all channels with the enabled front, peer pressed id.
// Sort SQL It's for Jean. UI With dispatcher See the same stable order..
func (d *DB) ListNotificationChannels(ctx context.Context) ([]*NotificationChannel, error) {
	rows, err := d.QueryContext(ctx, `SELECT `+notificationChannelCols+` FROM notification_channels
ORDER BY enabled DESC, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*NotificationChannel{}
	for rows.Next() {
		c, err := scanNotificationChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// NotificationChannelByID Access to individual channels.
func (d *DB) NotificationChannelByID(ctx context.Context, id int64) (*NotificationChannel, error) {
	row := d.QueryRowContext(ctx, `SELECT `+notificationChannelCols+` FROM notification_channels WHERE id=$1`, id)
	c, err := scanNotificationChannel(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotificationChannelNotFound
	}
	return c, err
}

// SaveNotificationChannel Create or update a new channel.
//
// Update only over the field given by the caller visible (no) nil / It's not empty, so the front end can submit local
// Modified drawer form without returning config Those fields in there it didn't show.——It's going to happen.
// [The mask covers the real key.]Accidents.
func (d *DB) SaveNotificationChannel(ctx context.Context, c *NotificationChannel) (int64, error) {
	if c.Mode == "" {
		c.Mode = NotifyModeRealtime
	}
	// This is the place.**No**Yes 0 Do any processing.:0 It's a legitimate configuration.[No current limit].
	//
	// Used to be written `if c.RatePerMin <= 0 { c.RatePerMin = Default value }`,It was meant to be.[When not specified
	// A security default.],But that's...[Preface As 0]They swallowed it together.——Document,UI Tips and
	// takeTokens Both 0 It's the only way to change it. 20(DingTalk/Micro/Telegram)
	// or 100(The operator thinks it's free. 20/The minutes are stuck and there's no hint..
	//
	// [Not specified]With[Visible 0]The difference is only known by the caller (the requested field is missing) vs Clear pass. 0),
	// So the default value by server Level filled when fields are defaulted. See notifyCreateChannel.
	if c.RatePerMin < 0 {
		return 0, errors.New("The limit value cannot be negative")
	}
	if c.Config == nil {
		c.Config = json.RawMessage(`{}`)
	}
	if c.Filter == nil {
		c.Filter = json.RawMessage(`{}`)
	}
	enabled := c.IsEnabled()

	if c.ID == 0 {
		var id int64
		err := d.QueryRowContext(ctx, `INSERT INTO notification_channels(name,kind,enabled,config,mode,filter,rate_per_min)
VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			c.Name, c.Kind, enabled, string(c.Config), c.Mode, string(c.Filter), c.RatePerMin).Scan(&id)
		return id, err
	}
	res, err := d.ExecContext(ctx, `UPDATE notification_channels
SET name=$2, kind=$3, enabled=$4, config=$5, mode=$6, filter=$7, rate_per_min=$8
WHERE id=$1`,
		c.ID, c.Name, c.Kind, enabled, string(c.Config), c.Mode, string(c.Filter), c.RatePerMin)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, ErrNotificationChannelNotFound
	}
	return c.ID, nil
}

// SetNotificationChannelEnabled Toggle departure.
//
// When a channel is disabled, mark the delivery that has not yet been sent as skipped:If not re-enabled
// We'll get a batch.[Backlog during decommissioning]The old loophole, the time limit is lost and can easily be miscalculated as new..
func (d *DB) SetNotificationChannelEnabled(ctx context.Context, id int64, enabled bool) error {
	return d.WithEvidenceTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE notification_channels SET enabled=$2 WHERE id=$1`, id, enabled)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotificationChannelNotFound
		}
		if !enabled {
			if _, err := tx.ExecContext(ctx, `UPDATE notification_deliveries SET state=$2, last_error=$3
WHERE channel_id=$1 AND state IN ($4,$5)`,
				id, NotifyStateSkipped, "Channel disabled", NotifyStatePending, NotifyStateSending); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteNotificationChannel Delete channel. Their delivery history deletes with external key cascades
// (There's no way to read history.).
func (d *DB) DeleteNotificationChannel(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `DELETE FROM notification_channels WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotificationChannelNotFound
	}
	return nil
}

// RecordNotificationEventTx In the caller's business.**Try.**Write a push event.
//
// This is the only notice-related change on the loophole: once INSERT,No watch, no channel.,
// Don't run the filter. It's a promise.[Hole Out]With[Send Task Exists]Atoms Same,
// There are no successful windows where messages are lost forever..
//
// Two key designs, not handwritten.:
//
//  1. **Why? SAVEPOINT**:PostgreSQL The word 'in-house' is wrong.
//     aborted status, all subsequent statements COMMIT)All fail. So...[Ignore this. INSERT
//     error, let the caller continue submitting]at PG Lee can't do it.——Unless you isolate the error with a preservation point.
//     This statement. There's no point, there's only one.[Roll Back]This option.
//
//  2. **Why is it wrong to roll back?**:Delivery is a function of convenience, and the record of loopholes is the product itself. One announcement
//     The problem with the table (unmoved old library, instantaneous disk failure) should not keep the high-risk loophole out of the library. So it's quarantined.
//     Error, log, return false,Let the loophole be written as usual——The price is to lose this delivery..
//     Return bool instead of error It's intentional: the caller shouldn't think of it as an error that affects success or failure..
func RecordNotificationEventTx(ctx context.Context, tx *sql.Tx, kind string, findingID int64, snap notify.Snapshot) bool {
	raw, err := json.Marshal(snap)
	if err != nil {
		log.Printf("[notify] Sequenced push event failed finding=%d: %v", findingID, err)
		return false
	}
	if _, err := tx.ExecContext(ctx, `SAVEPOINT notify_event`); err != nil {
		log.Printf("[notify] Failed to create saving point finding=%d: %v", findingID, err)
		return false
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO notification_events(kind,finding_id,snapshot) VALUES($1,$2,$3)`,
		kind, findingID, string(raw)); err != nil {
		log.Printf("[notify] Writing to push event failed finding=%d(The bug record is intact.): %v", findingID, err)
		// Roll back to the preservation point and get the business from aborted Save it in your state..
		if _, rbErr := tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT notify_event`); rbErr != nil {
			log.Printf("[notify] Failed to roll back to saving point finding=%d: %v", findingID, rbErr)
		}
		return false
	}
	// Release the saving point to avoid the accumulation of useless saving points in long business.
	_, _ = tx.ExecContext(ctx, `RELEASE SAVEPOINT notify_event`)
	return true
}

// AddNotificationEvent Yes InsertNotificationEventTx Independent service version for not available
// Call points in established transactions (e.g. channel)[Send test message],It's not real. finding).
func (d *DB) AddNotificationEvent(ctx context.Context, kind string, findingID int64, snap notify.Snapshot) (int64, error) {
	raw, err := json.Marshal(snap)
	if err != nil {
		return 0, fmt.Errorf("Sequenced notification event snapshot failed: %w", err)
	}
	var id int64
	err = d.QueryRowContext(ctx, `INSERT INTO notification_events(kind,finding_id,snapshot) VALUES($1,$2,$3) RETURNING id`,
		kind, findingID, string(raw)).Scan(&id)
	return id, err
}

// FanOutPendingEvents Commencing the currently active channel to deliver the outstanding bug event,
// Number of events returned to current cycle and new delivery.
//
// Round operation in one service: event use FOR UPDATE SKIP LOCKED Collect, multiple processes running simultaneously
// They also received different lines.
// db/task_archives.go of completeNextArchiveJob).
//
// Filter matching deliberately placed Go Side instead of Side SQL:Channel filter condition is a set of optional fields JSONB,
// Use SQL The matching of the six combinations makes the query difficult to maintain and the number of channels is[How many are there?],
// Once fully loaded, it's faster and easier to measure in the memory by article..
//
// Any failure to hit any channel will also be marked. fanned_out ——Otherwise, it'll stay in the pool forever.,
// each tick Sweeped again..
func (d *DB) FanOutPendingEvents(ctx context.Context, limit int) (eventCount, deliveryCount int, err error) {
	if limit <= 0 {
		limit = 200
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback() //nolint:errcheck // After successful submission no-op

	channels, err := listEnabledNotificationChannelsTx(ctx, tx)
	if err != nil {
		return 0, 0, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, kind, finding_id, snapshot FROM notification_events
WHERE NOT fanned_out ORDER BY id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, 0, err
	}
	var (
		events      []NotificationEvent
		parsedSnaps []notify.Snapshot
	)
	for rows.Next() {
		var ev NotificationEvent
		if err := rows.Scan(&ev.ID, &ev.Kind, &ev.FindingID, &ev.Snapshot); err != nil {
			rows.Close()
			return 0, 0, err
		}
		var snap notify.Snapshot
		// The snapshot was written by us, and it must be decipherable in theory; failure does not interrupt delivery.,
		// But this event will be bypassed by all filtering channels because the field is empty.——I'd rather not push one.
		// And don't let a bad card kill the whole line..
		_ = json.Unmarshal(ev.Snapshot, &snap)
		// kind Based on the inside of the line: the photo in the snapshot is a printed copy that could be written in the old version. Pass..
		snap.Kind = ev.Kind
		events = append(events, ev)
		parsedSnaps = append(parsedSnaps, snap)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}
	if len(events) == 0 {
		return 0, 0, tx.Commit()
	}

	type pending struct {
		eventID   int64
		channelID int64
	}
	var toInsert []pending
	for i, snap := range parsedSnaps {
		for _, ch := range channels {
			if !notify.Match(notify.ParseFilter(ch.Filter), snap) {
				continue
			}
			toInsert = append(toInsert, pending{eventID: events[i].ID, channelID: ch.ID})
		}
	}
	if len(toInsert) > 0 {
		var (
			vals []string
			args []any
		)
		for _, p := range toInsert {
			vals = append(vals, fmt.Sprintf("($%d,$%d)", len(args)+1, len(args)+2))
			args = append(args, p.eventID, p.channelID)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO notification_deliveries(event_id,channel_id) VALUES `+strings.Join(vals, ","), args...); err != nil {
			return 0, 0, err
		}
	}

	// Mark this round event has been assigned. Events that did not hit any channel are also marked together (see function comment)).
	ids := make([]string, 0, len(events))
	markArgs := make([]any, 0, len(events))
	for _, ev := range events {
		markArgs = append(markArgs, ev.ID)
		ids = append(ids, fmt.Sprintf("$%d", len(markArgs)))
	}
	if _, err := tx.ExecContext(ctx, `UPDATE notification_events SET fanned_out=true WHERE id IN (`+strings.Join(ids, ",")+`)`, markArgs...); err != nil {
		return 0, 0, err
	}
	return len(events), len(toInsert), tx.Commit()
}

// listEnabledNotificationChannelsTx Take the active channel in the transaction. Few.,
// No page breaks and no caches.——Cache will introduce[When will the configuration be effective?]This extra time series problem..
func listEnabledNotificationChannelsTx(ctx context.Context, tx *sql.Tx) ([]*NotificationChannel, error) {
	rows, err := tx.QueryContext(ctx, `SELECT id, name, kind, config, mode, filter, rate_per_min
FROM notification_channels WHERE enabled ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*NotificationChannel{}
	for rows.Next() {
		var c NotificationChannel
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Config, &c.Mode, &c.Filter, &c.RatePerMin); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// NotificationAssetNames Put assets id Parsing to short display names for uploading messages.
//
// Return order is consistent with participation and may be less than participation (non-existent) id Skipped. Keep the order of participation.
// In order to stabilize the sequence of assets in multiple deliveries of the same leak.——Otherwise, we'll try again.
// The asset sequence has changed. It'll be misread.[The assets have changed.].
func (d *DB) NotificationAssetNames(ctx context.Context, ids []int64) ([]string, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	ph, args := placeholders(1, ids)
	rows, err := d.QueryContext(ctx, `SELECT id, type, domain, ip, url, app_name, bundle_id FROM assets WHERE id IN (`+ph+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := map[int64]string{}
	for rows.Next() {
		var (
			id                int64
			typ               string
			domain, ip, url   sql.NullString
			appName, bundleID sql.NullString
		)
		if err := rows.Scan(&id, &typ, &domain, &ip, &url, &appName, &bundleID); err != nil {
			return nil, err
		}
		labels[id] = assetDisplayName(typ, domain.String, ip.String, url.String, appName.String, bundleID.String)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]string, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if label, ok := labels[id]; ok && label != "" {
			out = append(out, label)
		}
	}
	return out, nil
}

// assetDisplayName Select the most visible identifier by asset type.
// Back to the empty string at the bottom.[Assets not named]——This function does not assume placeholders,
// Otherwise[Assets#42]This noise is going to get in the mail. Readers think it's a real domain..
func assetDisplayName(typ, domain, ip, url, appName, bundleID string) string {
	pick := func(vals ...string) string {
		for _, v := range vals {
			if strings.TrimSpace(v) != "" {
				return v
			}
		}
		return ""
	}
	switch typ {
	case "root_domain", "subdomain":
		return domain
	case "ip":
		return ip
	case "app":
		return pick(appName, bundleID)
	case "service", "endpoint":
		return pick(url, domain, ip)
	default:
		return pick(domain, ip, url, appName)
	}
}

// SetFindingStatusWithNotify Update gap disposal status and register a change in status in the same service
// Organisation.
//
// Return from=Status Before Change;found=Is there a loophole?;notified=Events registered successfully.
//
// Three deliberate acts.:
//   - Events are not registered when the status has not changed. Resubmission of same value or automated script in front drawer
//     We can't even produce noise..
//   - Return when the loophole does not exist found=false without writing anything, translated by the caller 404.
//   - The failure to register events does not affect the status update (see RecordNotificationEventTx Can not open message),
//     So notified=false The situation has changed. The caller should not be mistaken..
func (d *DB) SetFindingStatusWithNotify(ctx context.Context, id int64, status string) (from string, found bool, notified bool, err error) {
	err = d.WithEvidenceTx(ctx, func(tx *sql.Tx) error {
		var txErr error
		from, found, _, notified, txErr = SetFindingStatusTx(ctx, tx, id, status)
		return txErr
	})
	return from, found, notified, err
}

// SetFindingStatusTx at**Caller services**Update bug status inside and register status change push events.
//
// Draws into a service level function so that all changed paths share the same semantics——It was just...
// patchFinding Go with the notice version, and**Reconciling conclusion:[Fixed]hour**(finding_retests
// The one in there. `UPDATE findings SET status=...`)It's a direct library, and it's a match.
// `on_status_change` The channel is completely unreceivable for this state flow: the state on the interface has changed quietly.,
// It's not until we open the platform..
//
// Return from=Pre-change state,found=Is there a loophole?,changed=Has the state really changed?,
// notified=Whether or not the event was registered successfully (the failure does not affect the status update; see RecordNotificationEventTx).
func SetFindingStatusTx(ctx context.Context, tx *sql.Tx, id int64, status string) (from string, found bool, changed bool, notified bool, err error) {
	var (
		vulnclass, name, severity, summary string
		taskID                             sql.NullInt64
		assetIDs                           []byte
	)
	scanErr := tx.QueryRowContext(ctx, `SELECT vulnclass, name, severity, summary, task_id, asset_ids, status
FROM findings WHERE id=$1 FOR UPDATE`, id).
		Scan(&vulnclass, &name, &severity, &summary, &taskID, &assetIDs, &from)
	if scanErr == sql.ErrNoRows {
		return "", false, false, false, nil
	}
	if scanErr != nil {
		return "", false, false, false, scanErr
	}
	found = true
	if from == status {
		// Non-registration of events without a real change in status: duplicate submission of the same value, reset of the thorium, etc.
		// It's making noise..
		return from, true, false, false, nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE findings SET status=$2 WHERE id=$1`, id, status); err != nil {
		return from, true, false, false, err
	}
	var assets []int64
	_ = json.Unmarshal(assetIDs, &assets)
	notified = RecordNotificationEventTx(ctx, tx, notify.EventFindingStatusChanged, id, notify.Snapshot{
		Kind:       notify.EventFindingStatusChanged,
		FindingID:  id,
		TaskID:     taskID.Int64,
		VulnClass:  vulnclass,
		Name:       name,
		Severity:   severity,
		Summary:    summary,
		AssetIDs:   assets,
		FromStatus: from,
		ToStatus:   status,
	})
	return from, true, true, notified, nil
}

// NotificationStats is the overview count at the top of the notification page.
type NotificationStats struct {
	Channels     int   `json:"channels"`
	ChannelsOn   int   `json:"channels_on"`
	Pending      int   `json:"pending"`
	Failed       int   `json:"failed"`
	SentToday    int   `json:"sent_today"`
	BacklogAgeMS int64 `json:"backlog_age_ms"` // The oldest to be delivered in milliseconds
}

// NotificationStatsSnapshot Health of the aggregate notification system.
// BacklogAgeMS Yes[Is it stuck?]Most immediate indicators——That's right. pending Counting is much more useful.,
// Because of the backlog. 3 Article and backlog 3 The difference in the article can be from 3 Seconds to arrive 3 hours.
func (d *DB) NotificationStatsSnapshot(ctx context.Context) (*NotificationStats, error) {
	var s NotificationStats
	if err := d.QueryRowContext(ctx, `SELECT
    (SELECT count(*) FROM notification_channels),
    (SELECT count(*) FROM notification_channels WHERE enabled),
    (SELECT count(*) FROM notification_deliveries WHERE state IN ($1,$2)),
    (SELECT count(*) FROM notification_deliveries WHERE state=$3),
    (SELECT count(*) FROM notification_deliveries WHERE state=$4 AND sent_at >= date_trunc('day', now())),
    COALESCE((SELECT EXTRACT(EPOCH FROM (now() - min(created_at))) * 1000 FROM notification_deliveries WHERE state=$1), 0)::bigint`,
		NotifyStatePending, NotifyStateSending, NotifyStateFailed, NotifyStateSent).
		Scan(&s.Channels, &s.ChannelsOn, &s.Pending, &s.Failed, &s.SentToday, &s.BacklogAgeMS); err != nil {
		return nil, err
	}
	return &s, nil
}
