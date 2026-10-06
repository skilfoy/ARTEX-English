package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// This file claims delivery tasks and moves them through states.
//
// Claiming uses a lease, not a long transaction: the row is set to sending and
// next_attempt_at is pushed into the future as the lease expiry. The
// transaction commits before the network send, so delivery does not hold a
// database lock. A request can take several seconds (the client timeout is
// 15 seconds); holding the row lock would stall other writes on the same database.
//
// If the process crashes mid-delivery, the row stays in sending. That heals
// itself: once the lease expires, next_attempt_at is in the past and the next
// claim picks the same row up again (the claim predicate includes
// state IN ('pending','sending')). The attempt counter is incremented at claim
// time, so a crash cannot retry forever. After MaxNotifyAttempts the row lands
// in failed and waits for a person.

// MaxNotifyAttempts is the maximum number of attempts for one delivery, including the first.
// It lives here, not in the delivery engine: it is the state machine's policy, and the engine only executes it.
const MaxNotifyAttempts = 3

// MaxDigestBatchSize is how many deliveries one digest batch may merge.
//
// The bound exists for resource reasons. One digest cycle can see tens of
// thousands of findings (a full scan is enough). Without a cap, the claim would
// read every row into memory, render one huge message, and then have the
// channel length limit cut most of it off. That wastes memory and silently
// drops the findings that were truncated. With the cap, the overflow stays in
// the database as the next batch and goes out on the next cycle.
//
// 500 is the size that still leaves readable content inside WeCom's 4096-byte
// limit after rendering. Anything larger only moves the truncation point later.
const MaxDigestBatchSize = 500

// NotificationDelivery is one delivery task, including the channel config and event snapshot needed to render it.
type NotificationDelivery struct {
	ID            int64           `json:"id"`
	EventID       int64           `json:"event_id"`
	ChannelID     int64           `json:"channel_id"`
	State         string          `json:"state"`
	Attempts      int             `json:"attempts"`
	NextAttemptAt time.Time       `json:"next_attempt_at"`
	LastError     string          `json:"last_error"`
	BatchID       *int64          `json:"batch_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
	SentAt        *time.Time      `json:"sent_at,omitempty"`
	Snapshot      json.RawMessage `json:"snapshot,omitempty"`
	// Rendering context loaded with the row. Not serialized; the server layer builds the DTO.
	Channel *NotificationChannel `json:"-"`
	// FindingID and EventKind come from the event so the history list can open the finding.
	FindingID int64  `json:"finding_id,string"`
	EventKind string `json:"event_kind"`
	// ChannelName and ChannelKind are denormalized for the list view so the frontend does not query again.
	ChannelName string `json:"channel_name"`
	ChannelKind string `json:"channel_kind"`
}

const notificationDeliveryCols = `d.id, d.event_id, d.channel_id, d.state, d.attempts, d.next_attempt_at,
       d.last_error, d.batch_id, d.created_at, d.sent_at`

// joinedDeliveryQuery is the shared read shape: delivery row, event snapshot, and channel config.
// Rendering a message needs all three; splitting them would be three round trips.
const joinedDeliveryQuery = `SELECT ` + notificationDeliveryCols + `,
       e.snapshot, e.kind, e.finding_id,
       c.id, c.name, c.kind, c.enabled, c.config, c.mode, c.filter, c.rate_per_min
FROM notification_deliveries d
JOIN notification_events e ON e.id = d.event_id
JOIN notification_channels c ON c.id = d.channel_id`

func scanNotificationDelivery(sc interface{ Scan(...any) error }) (*NotificationDelivery, error) {
	var (
		dl        NotificationDelivery
		lastErr   sql.NullString
		batchID   sql.NullInt64
		sentAt    sql.NullTime
		snapshot  []byte
		eventKind string
		channel   NotificationChannel
		chEnabled bool
	)
	if err := sc.Scan(&dl.ID, &dl.EventID, &dl.ChannelID, &dl.State, &dl.Attempts, &dl.NextAttemptAt,
		&lastErr, &batchID, &dl.CreatedAt, &sentAt,
		&snapshot, &eventKind, &dl.FindingID,
		&channel.ID, &channel.Name, &channel.Kind, &chEnabled, &channel.Config, &channel.Mode, &channel.Filter, &channel.RatePerMin); err != nil {
		return nil, err
	}
	dl.LastError = lastErr.String
	if batchID.Valid {
		dl.BatchID = &batchID.Int64
	}
	if sentAt.Valid {
		dl.SentAt = &sentAt.Time
	}
	dl.Snapshot = json.RawMessage(snapshot)
	dl.EventKind = eventKind
	dl.ChannelName = channel.Name
	dl.ChannelKind = channel.Kind
	channel.Enabled = &chEnabled
	dl.Channel = &channel
	return &dl, nil
}

// claimQuery describes one claim: select candidates with sel and lock them,
// then mark them sending and extend the lease. The caller supplies the lease
// argument in the $n placeholder inside sel.
type claimQuery struct {
	sql  string
	args []any
}

// ClaimRealtimeDeliveries claims up to limit due realtime deliveries for one channel.
//
// Claims are per channel, not "take a global batch and then pick what to send".
// The delivery engine's rate-limit gate is per channel. The engine must know
// how many this channel may still send this round, then claim exactly that many
// rows, or the limit would burn retry attempts. Claiming first and discarding
// the rest would already have incremented attempts on rows that were only
// waiting. The budget of 3 would be spent on idle waits and the rows would land in failed.
//
// The predicate includes sending rows whose lease has expired. That is how a
// crash heals. lease must be well above the worst-case send time (the channel
// HTTP client times out at 15 seconds), or two dispatchers will deliver the
// same row. Disabled channels are excluded too: disabling already marks
// existing deliveries skipped, and this is a second check so a disable racing
// a claim cannot slip through.
func (d *DB) ClaimRealtimeDeliveries(ctx context.Context, channelID int64, limit int, lease time.Duration) ([]*NotificationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	return d.claimDeliveries(ctx, lease, claimQuery{
		sql: `SELECT dd.id FROM notification_deliveries dd
JOIN notification_channels c ON c.id = dd.channel_id
WHERE dd.channel_id = $1 AND dd.state IN ($2,$3) AND dd.next_attempt_at <= now()
  AND c.enabled AND c.mode = $4
ORDER BY dd.next_attempt_at, dd.id
FOR UPDATE OF dd SKIP LOCKED
LIMIT $5`,
		args: []any{channelID, NotifyStatePending, NotifyStateSending, NotifyModeRealtime, limit},
	}, nil)
}

// DigestBatchDue reports whether the channel has a due digest batch: there is
// a pending delivery, and the oldest one's age has reached the digest period.
//
// The decision uses the oldest delivery's age, not the wall clock. A channel
// that was just created will not immediately emit a one-item "digest" because
// the clock rolled to the hour, and a long-backlogged batch will not sit
// through another idle cycle.
//
// This is separate from ClaimDigestBatch because the meanings differ. This
// function only answers "should we send". The claim takes every pending
// delivery for the channel, including ones that are not old enough yet.
// Otherwise one period would be split into several messages and the digest
// would be pointless.
func (d *DB) DigestBatchDue(ctx context.Context, channelID int64, minAge time.Duration) (bool, error) {
	var due bool
	err := d.QueryRowContext(ctx, `SELECT EXISTS (
  SELECT 1 FROM notification_deliveries d
  JOIN notification_channels c ON c.id = d.channel_id
  WHERE d.channel_id = $1 AND d.state IN ($2,$3) AND c.enabled
  GROUP BY d.channel_id
  HAVING min(d.created_at) <= now() - make_interval(secs => $4)
)`, channelID, NotifyStatePending, NotifyStateSending, int64(minAge.Seconds())).Scan(&due)
	return due, err
}

// ClaimDigestBatch claims a channel's currently due pending deliveries as one
// digest batch, at most MaxDigestBatchSize rows.
//
// Every delivery in the batch shares batch_id. The smallest id in the set is
// the batch number: stable, readable, and no extra sequence. Retries keep the
// original batch id with COALESCE, so "these N rows were sent together" still
// holds after several attempts.
//
// The first N rows are taken in id order, not at random. The oldest deliveries
// go first, so a backlog cannot starve old findings behind newer ones.
func (d *DB) ClaimDigestBatch(ctx context.Context, channelID int64, limit int, lease time.Duration) ([]*NotificationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	// limit is a memory cap. Callers pass MaxDigestBatchSize; this clamps
	// anything larger.
	//
	// Do not pass a rate-limit allowance as the batch size. The rate limit
	// counts messages: one batch is one message and costs one token, deducted
	// by takeTokens in the server layer. That is a different unit from "how
	// many findings fit in a batch". Passing the per-round request budget in
	// so that rate_per_min would apply to digest made a 20/min channel pack
	// only one finding per batch, and digest degraded into a realtime push
	// with digest wording. Change takeTokens' want to change the rate limit;
	// do not change this.
	if limit > MaxDigestBatchSize {
		limit = MaxDigestBatchSize
	}
	out, err := d.claimDeliveries(ctx, lease, claimQuery{
		sql: `SELECT dd.id FROM notification_deliveries dd
JOIN notification_channels c ON c.id = dd.channel_id
WHERE dd.channel_id = $1 AND dd.state IN ($2,$3) AND dd.next_attempt_at <= now() AND c.enabled
ORDER BY dd.id
FOR UPDATE OF dd SKIP LOCKED
LIMIT $4`,
		args: []any{channelID, NotifyStatePending, NotifyStateSending, limit},
	}, func(tx *sql.Tx, ids []int64) error {
		batchID := ids[0]
		for _, id := range ids {
			if id < batchID {
				batchID = id
			}
		}
		ph, idArgs := placeholders(2, ids)
		_, err := tx.ExecContext(ctx, `UPDATE notification_deliveries SET batch_id = COALESCE(batch_id, $1)
WHERE id IN (`+ph+`)`, append([]any{batchID}, idArgs...)...)
		return err
	})
	return out, err
}

// claimDeliveries selects rows, marks them sending, extends the lease, and
// reads the full rows, all in one transaction. postClaim is an optional extra
// step; digest batches use it to write batch_id.
func (d *DB) claimDeliveries(ctx context.Context, lease time.Duration, cq claimQuery, postClaim func(*sql.Tx, []int64) error) ([]*NotificationDelivery, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful commit

	ids, err := selectForClaim(ctx, tx, cq.sql, cq.args...)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit()
	}
	// Mark sending and push next_attempt_at into the future. That timestamp is
	// the lease expiry, so "lease still valid" and "not yet time to retry"
	// share one predicate and no extra column is needed.
	ph, idArgs := placeholders(3, ids)
	if _, err := tx.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, attempts=attempts+1, next_attempt_at=now()+make_interval(secs => $2)
WHERE id IN (`+ph+`)`,
		append([]any{NotifyStateSending, lease.Seconds()}, idArgs...)...); err != nil {
		return nil, err
	}
	if postClaim != nil {
		if err := postClaim(tx, ids); err != nil {
			return nil, err
		}
	}
	out, err := loadDeliveriesTx(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	return out, tx.Commit()
}

func selectForClaim(ctx context.Context, tx *sql.Tx, query string, args ...any) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func loadDeliveriesTx(ctx context.Context, tx *sql.Tx, ids []int64) ([]*NotificationDelivery, error) {
	ph, args := placeholders(1, ids)
	rows, err := tx.QueryContext(ctx, joinedDeliveryQuery+` WHERE d.id IN (`+ph+`) ORDER BY d.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*NotificationDelivery{}
	for rows.Next() {
		dl, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dl)
	}
	return out, rows.Err()
}

// MarkDeliveriesSent marks a batch of deliveries as sent.
func (d *DB) MarkDeliveriesSent(ctx context.Context, ids []int64) error {
	ph, args := placeholders(2, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, sent_at=now(), last_error='' WHERE id IN (`+ph+`)`, append([]any{NotifyStateSent}, args...)...)
	return err
}

// RescheduleDeliveries returns a batch of deliveries to pending and delays the next retry.
//
// They go back to pending instead of a new intermediate state so "attempts
// remaining" is expressed in only one place (MaxNotifyAttempts). The state
// machine does not grow a branch for every retry policy.
func (d *DB) RescheduleDeliveries(ctx context.Context, ids []int64, delay time.Duration, errMsg string) error {
	ph, args := placeholders(4, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, next_attempt_at=now()+make_interval(secs => $2), last_error=$3
WHERE id IN (`+ph+`)`,
		append([]any{NotifyStatePending, delay.Seconds(), truncateNotifyError(errMsg)}, args...)...)
	return err
}

// DeferDeliveries returns a batch to pending so it can be claimed immediately,
// and undoes the attempt counted when it was claimed.
//
// The only use is digest segmentation: when a digest is split to fit the
// channel length limit, items that did not fit this message wait for the next
// batch. That is not a failure, so it must not spend retry budget. Claiming
// already incremented attempts optimistically, and this decrements it again.
// Otherwise a backlog of 500, cut into 25 segments of 20, would mark the tail
// failed at segment 3 via MaxNotifyAttempts even though those rows never erred.
//
// GREATEST(..., 0) covers a manual resend that already zeroed attempts and
// then reached this path, so the counter cannot go negative.
func (d *DB) DeferDeliveries(ctx context.Context, ids []int64, reason string) error {
	ph, args := placeholders(3, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, attempts=GREATEST(attempts-1, 0), next_attempt_at=now(), last_error=$2
WHERE id IN (`+ph+`)`,
		append([]any{NotifyStatePending, truncateNotifyError(reason)}, args...)...)
	return err
}

// FailDeliveries marks a batch as permanently failed. A person can resend them from delivery history.
func (d *DB) FailDeliveries(ctx context.Context, ids []int64, errMsg string) error {
	// Placeholders start at $3: $1 is state, $2 is last_error.
	ph, args := placeholders(3, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries SET state=$1, last_error=$2 WHERE id IN (`+ph+`)`,
		append([]any{NotifyStateFailed, truncateNotifyError(errMsg)}, args...)...)
	return err
}

// RetryNotificationDelivery manually resends one delivery: reset it to pending,
// clear the attempt count, and make it due immediately. Clearing the count is
// deliberate. A manual resend means the earlier failures have been handled,
// so the old count should not keep limiting it.
func (d *DB) RetryNotificationDelivery(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$2, attempts=0, next_attempt_at=now(), last_error=''
WHERE id=$1 AND state IN ($3,$4)`, id, NotifyStatePending, NotifyStateFailed, NotifyStateSkipped)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("delivery %d does not exist or its current state does not allow a resend", id)
	}
	return nil
}

// NotificationDeliveryFilter is the query for delivery history.
type NotificationDeliveryFilter struct {
	ChannelID int64
	State     string
	EventKind string
}

func (f NotificationDeliveryFilter) where() (string, []any) {
	var conds []string
	var args []any
	if f.ChannelID > 0 {
		args = append(args, f.ChannelID)
		conds = append(conds, fmt.Sprintf("d.channel_id=$%d", len(args)))
	}
	if f.State != "" {
		args = append(args, f.State)
		conds = append(conds, fmt.Sprintf("d.state=$%d", len(args)))
	}
	if f.EventKind != "" {
		args = append(args, f.EventKind)
		conds = append(conds, fmt.Sprintf("e.kind=$%d", len(args)))
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// ListNotificationDeliveries returns delivery history one page at a time, newest first.
func (d *DB) ListNotificationDeliveries(ctx context.Context, f NotificationDeliveryFilter, page, pageSize int) ([]*NotificationDelivery, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 50
	}
	where, args := f.where()

	var total int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM notification_deliveries d
JOIN notification_events e ON e.id = d.event_id`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := fmt.Sprintf("%s%s ORDER BY d.id DESC LIMIT $%d OFFSET $%d",
		joinedDeliveryQuery, where, len(args)+1, len(args)+2)
	rows, err := d.QueryContext(ctx, q, append(args, pageSize, (page-1)*pageSize)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*NotificationDelivery{}
	for rows.Next() {
		dl, err := scanNotificationDelivery(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, dl)
	}
	return out, total, rows.Err()
}

// truncateNotifyError cuts an error to a length the column can store. A channel
// response body can be long, especially a generic webhook hitting a custom
// service, and leaving it uncut would bloat the history list payload.
func truncateNotifyError(msg string) string {
	const max = 500
	if len(msg) <= max {
		return msg
	}
	// Step back to a character boundary so a split UTF-8 sequence is not left for the frontend to render as garbage.
	cut := max
	for cut > 0 && !isUTF8Start(msg[cut]) {
		cut--
	}
	return msg[:cut] + "…"
}

func isUTF8Start(b byte) bool { return b&0xC0 != 0x80 }

// placeholders builds $n placeholders starting at start, plus the matching args, for IN (...).
// For example start=3, ids=[7,8] yields "$3,$4" and [7,8].
func placeholders(start int, ids []int64) (string, []any) {
	ph := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		ph = append(ph, fmt.Sprintf("$%d", start+i))
		args = append(args, id)
	}
	return strings.Join(ph, ","), args
}
