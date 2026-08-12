package main

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
)

// Handler reacts to incoming decrypted m.room.message events and sends the
// configured auto-reply, at most once per sender per day, only in direct
// message rooms, and only once the bot is actually caught up (not replaying
// history from before it started).
type Handler struct {
	client      *mautrix.Client
	dmTracker   *DMTracker
	rateLimiter *RateLimiter
	replyText   string
	startedAt   int64 // unix millis, matches event.Timestamp's units
	log         zerolog.Logger

	firstSyncDone atomic.Bool
}

func NewHandler(client *mautrix.Client, dmTracker *DMTracker, rateLimiter *RateLimiter, replyText string, log zerolog.Logger) *Handler {
	return &Handler{
		client:      client,
		dmTracker:   dmTracker,
		rateLimiter: rateLimiter,
		replyText:   replyText,
		startedAt:   time.Now().UnixMilli(),
		log:         log,
	}
}

// MarkFirstSyncDone should be called once the first /sync round-trip has
// completed, so the handler stops ignoring events (see HandleMessage).
func (h *Handler) MarkFirstSyncDone() {
	h.firstSyncDone.Store(true)
}

// HandleMessage is a syncer event handler for event.EventMessage. Encrypted
// messages arrive here too, already decrypted by the crypto helper.
func (h *Handler) HandleMessage(ctx context.Context, evt *event.Event) {
	log := h.log.With().
		Str("event_id", evt.ID.String()).
		Str("room_id", evt.RoomID.String()).
		Str("sender", evt.Sender.String()).
		Logger()

	if evt.Sender == h.client.UserID {
		// Never reply to our own messages (including our own previous
		// auto-replies) - this is what stops the bot talking to itself.
		return
	}

	if !h.firstSyncDone.Load() || evt.Timestamp < h.startedAt {
		// Ignore timeline backlog delivered as part of the very first sync,
		// and anything timestamped before this process started.
		log.Debug().Msg("Ignoring message from before bot start")
		return
	}

	if !h.dmTracker.IsDM(evt.RoomID) {
		return
	}

	content := evt.Content.AsMessage()
	if content.MsgType != event.MsgText {
		// Skip notices (the standard way bots mark their own messages to
		// avoid loops), emotes, images, etc.
		return
	}

	if !h.rateLimiter.ShouldReply(evt.Sender) {
		log.Debug().Msg("Already replied to this sender today, skipping")
		return
	}

	names := resolveSenderNames(ctx, h.client, evt.RoomID, evt.Sender)
	reply := renderMessage(h.replyText, names)

	log.Info().Str("first_name", names.FirstName).Msg("Sending absence auto-reply")
	if _, err := h.client.SendText(ctx, evt.RoomID, reply); err != nil {
		log.Error().Err(err).Msg("Failed to send auto-reply")
		return
	}

	if err := h.rateLimiter.MarkReplied(evt.Sender); err != nil {
		log.Error().Err(err).Msg("Failed to persist rate limiter state")
	}
}
