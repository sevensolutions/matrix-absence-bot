package main

import (
	"context"
	"sync"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/id"
)

// DMTracker keeps track of which rooms are direct-message rooms, based on
// the account's "m.direct" account data. It is populated once at startup
// and then kept live via a sync handler, so newly-created DMs are picked up
// without restarting the bot.
type DMTracker struct {
	mu    sync.RWMutex
	rooms map[id.RoomID]bool
}

func NewDMTracker() *DMTracker {
	return &DMTracker{rooms: make(map[id.RoomID]bool)}
}

// Load fetches the current "m.direct" account data from the server and
// (re)builds the room set from it.
func (t *DMTracker) Load(ctx context.Context, client *mautrix.Client) error {
	var content event.DirectChatsEventContent
	err := client.GetAccountData(ctx, event.AccountDataDirectChats.Type, &content)
	if err != nil {
		return err
	}
	t.setFromContent(content)
	return nil
}

// HandleAccountData is a sync event handler for live "m.direct" updates.
func (t *DMTracker) HandleAccountData(_ context.Context, evt *event.Event) {
	content := evt.Content.AsDirectChats()
	if content == nil {
		return
	}
	t.setFromContent(*content)
}

func (t *DMTracker) setFromContent(content event.DirectChatsEventContent) {
	rooms := make(map[id.RoomID]bool)
	for _, roomIDs := range content {
		for _, roomID := range roomIDs {
			rooms[roomID] = true
		}
	}

	t.mu.Lock()
	t.rooms = rooms
	t.mu.Unlock()
}

// IsDM reports whether roomID is a known direct-message room.
func (t *DMTracker) IsDM(roomID id.RoomID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rooms[roomID]
}
