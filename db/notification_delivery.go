package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// This document is the recipient and status flow of the delivery task.
//
// Receipt[Leases]It's not a long story. sending And put next_attempt_at To the future.
// The lease expires when the services are submitted for delivery. Do not hold database locks during delivery——
// Network requests may take several seconds (client timeout) 15 In seconds, holding the line lock will drag down the other writing operations of the library..
//
// The price is if the process crashes on the way to delivery, the line stops. sending.This is...**Healing himself.**After expiration of lease
// next_attempt_at If you fall in the past, the next round will retake the same line.
// state IN ('pending','sending')).Retry count when it's collected +1,So the crash won't cause it.
// Unlimited Retry——MaxNotifyAttempts Once you've had your chance, you fall in. failed Waiting for manual processing.

// MaxNotifyAttempts is the maximum number of attempts of a delivery).
// Define here, not in the delivery engine: it's a state machine's own strategy, the engine's an implementer..
const MaxNotifyAttempts = 3

// MaxDigestBatchSize is the maximum number of individual batches merged at once.
//
// The raison d ' être is resources: if tens of thousands of loopholes are addressed in an aggregate cycle (opposable)——A full scan.
// If you don't set the upper bounds, you can read all the lines into the memory.,
// And then they cut off half the length limit of the channel.——It's a waste of memory.**Quietly lost.**The holes that were blocked..
// Once the line is set, the excess remains in the vault for the next batch, and the next cycle will be natural..
//
// take 500 It's based on the fact that after the news is published, 4096 The byte limit is within"Readable."Scale;
// It's bigger than that. It's just that the cut-off takes place further behind..
const MaxDigestBatchSize = 500

// NotificationDelivery It's a delivery mission with the channel configuration and event snapshot required for rendering..
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
	// Joint load rendering context, no JSON(By server Layer assembly DTO).
	Channel *NotificationChannel `json:"-"`
	// FindingID/EventKind Take it out of the event for the history list to jump through the loophole..
	FindingID int64  `json:"finding_id,string"`
	EventKind string `json:"event_kind"`
	// ChannelName/ChannelKind is the redundant field for the list display, save the front-end double query.
	ChannelName string `json:"channel_name"`
	ChannelKind string `json:"channel_kind"`
}

const notificationDeliveryCols = `d.id, d.event_id, d.channel_id, d.state, d.attempts, d.next_attempt_at,
       d.last_error, d.batch_id, d.created_at, d.sent_at`

// joinedDeliveryQuery is the unified reading shape of the delivery line: delivery + Eventshot + Channel Configuration.
// If you can't read a message, it'll be three trips..
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

// claimQuery Description of the receipt: Press first sel Select the candidates and lock them up. sending and
// Extension of the lease.sel inside lease Location by Caller $n I'm in position and I'm in..
type claimQuery struct {
	sql  string
	args []any
}

// ClaimRealtimeDeliveries Receipt of a shipment due in real time from a certain channel, up to limit strip.
//
// Do it deliberately.**Single channel**To receive instead of[We'll take one and we'll pick up the hair.]:The portal's in the delivery engine.
// Maintenance. Only if we know how many more lines this channel can run and how many more lines we can take.
// Number of retries. If the other way around, it's already been counted once. attempts,
// 3 The next budget will be spent just waiting for it to come in. failed.
//
// Organisation[Leases expired sending]——It's a place of collapse..lease Must be significantly greater than single times
// Worst time for delivery (channel) HTTP Client timeout 15 Two in the same line. dispatcher
// Together. Also block the disabled channel: the disablement has marked the stock delivery as skipped,
// Here's another one, to avoid the drop-off and the drop-off..
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

// DigestBatchDue Report whether the channel has saved up to one due instalment: there is a pending delivery, and**The oldest one.**
// Age has reached summary cycle.
//
// The decision is based on the age of the oldest delivery, not on the wall clock: This new channel doesn't have to be aligned.
// I'm gonna spit out a single one right now.[Summary],A long backlog of batches will not wait for another round..
//
// With ClaimDigestBatch Separate because semantics differ: This function answers only[Should I?],
// And the money's gonna take it away.**All**To be issued (including those under age))——Or one cycle
// They'll be broken down into multiple pieces of information..
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

// ClaimDigestBatch Receiving a pending delivery due from a certain channel as a consolidated batch,
// Single maximum MaxDigestBatchSize strip.
//
// All deliveries of the same batch are shared batch_id,With the smallest of the collections id Batch numbers (stable, readable),
// No additional sequence is required. Use when retrying COALESCE Keep the original batch number so that[This one. N The strips were sent together.]
// It's still there after several attempts..
//
// Press id Before ascending N strips rather than random: the earliest deliveries are sent first and the backlog does not appear
// [New holes start first, old ones stay behind.]Hunger.
func (d *DB) ClaimDigestBatch(ctx context.Context, channelID int64, limit int, lease time.Duration) ([]*NotificationDelivery, error) {
	if limit <= 0 {
		return nil, nil
	}
	// limit Yes**Memory Upper**,Caller Fax MaxDigestBatchSize;Here, one more.,
	// Prevents the caller from moving into a larger value.
	//
	// I wouldn't accept it.[Flow limit]Be a batch size: the limited flow is the number of messages——One batch only
	// A message. A token. server Layer takeTokens Less——With[A bunch of them.
	// Vulnerability]It's two different scales. ♪ Once for Jean ♪ rate_per_min Yes digest Entry into force for each round
	// Request budget to be sent in for batch size, result rate=20/min Every batch of channels 1 A loophole,
	// digest It's degraded into a real-time transfer with a summary file. To change the flow, please. takeTokens of want,
	// Don't move here..
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

// claimDeliveries Execute[Select + set sending Extension of the lease + Read full lines],It's all a matter of business..
// postClaim It's an optional additional step (sum of batches written) batch_id).
func (d *DB) claimDeliveries(ctx context.Context, lease time.Duration, cq claimQuery, postClaim func(*sql.Tx, []int64) error) ([]*NotificationDelivery, error) {
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // After successful submission no-op

	ids, err := selectForClaim(ctx, tx, cq.sql, cq.args...)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, tx.Commit()
	}
	// set sending And put next_attempt_at Push to the future: this is the moment of the coming lease.,
	// [Lease not due]With[Not time to try again]Therefore, we share the same condition and do not need a new addition..
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

// MarkDeliveriesSent Can not open message.
func (d *DB) MarkDeliveriesSent(ctx context.Context, ids []int64) error {
	ph, args := placeholders(2, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$1, sent_at=now(), last_error='' WHERE id IN (`+ph+`)`, append([]any{NotifyStateSent}, args...)...)
	return err
}

// RescheduleDeliveries Return the delivery. pending And then try again later..
//
// Return pending Instead of introducing a new middle state, it's about letting[There are still a few chances.]Just one place.
// Expression(MaxNotifyAttempts),Avoids the branching of the status machine expanding with the retry strategy.
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

// DeferDeliveries Return the delivery. pending,immediate retake, and**The attempt to cancel the receipt count.**.
//
// There is only one use: when the aggregate message is sent by the maximum length of the channel, the entry that is not included in this article is left to the next Batch.
// It wasn't a failure, so it shouldn't cost the budget.——When received attempts Already optimistic. +1 Yeah.,
// It has to be down here. Or one. 500 The backlog of articles will follow each paragraph 20 Strip 25 section,
// End entry in 3 It's just a part of it. MaxNotifyAttempts Agreed. failed,And they never did anything wrong..
//
// GREATEST(...,0) Hold on.[Someone's hand-held. attempts And then it came here.]Situation,
// Don't let the count turn negative..
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

// FailDeliveries Mark the delivery as final failure, waiting for manual weight in the delivery history Fire!.
func (d *DB) FailDeliveries(ctx context.Context, ids []int64, errMsg string) error {
	// Placeholder From $3 Start:$1 Yes state,$2 Yes last_error.
	ph, args := placeholders(3, ids)
	if len(args) == 0 {
		return nil
	}
	_, err := d.ExecContext(ctx, `UPDATE notification_deliveries SET state=$1, last_error=$2 WHERE id IN (`+ph+`)`,
		append([]any{NotifyStateFailed, truncateNotifyError(errMsg)}, args...)...)
	return err
}

// RetryNotificationDelivery Redeal a delivery manually: Reset to pending,Zero test count,
// Due immediately. Counting is deliberate.——Manual[Resend]Which means that the reasons for previous failures have been addressed.,
// It doesn't make sense with the old count..
func (d *DB) RetryNotificationDelivery(ctx context.Context, id int64) error {
	res, err := d.ExecContext(ctx, `UPDATE notification_deliveries
SET state=$2, attempts=0, next_attempt_at=now(), last_error=''
WHERE id=$1 AND state IN ($3,$4)`, id, NotifyStatePending, NotifyStateFailed, NotifyStateSkipped)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("Organisation %d Cannot initialise Evolution's mail component.", id)
	}
	return nil
}

// NotificationDeliveryFilter It's a historical query condition..
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

// ListNotificationDeliveries Page Break Back to Drop History, New Before.
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

// truncateNotifyError Intercepts the error information to the acceptable length of the column. The channel's returned response may be long.
// (General Webhook It's especially when you hit a self-contained service..
func truncateNotifyError(msg string) string {
	const max = 500
	if len(msg) <= max {
		return msg
	}
	// Rewind by character boundary to avoid leaving half. UTF-8 Characters make the front end uncoded.
	cut := max
	for cut > 0 && !isUTF8Start(msg[cut]) {
		cut--
	}
	return msg[:cut] + "…"
}

func isUTF8Start(b byte) bool { return b&0xC0 != 0x80 }

// placeholders Generate From start Started. $n Positioning strings and corresponding parameters for IN (...) Use.
// For example start=3, ids=[7,8] → "$3,$4", [7,8].
func placeholders(start int, ids []int64) (string, []any) {
	ph := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for i, id := range ids {
		ph = append(ph, fmt.Sprintf("$%d", start+i))
		args = append(args, id)
	}
	return strings.Join(ph, ","), args
}
