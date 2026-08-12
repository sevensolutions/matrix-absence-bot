package main

import (
	"context"
	"strings"

	"maunium.net/go/mautrix"
	"maunium.net/go/mautrix/id"
)

// senderNames resolves the placeholder values available for a given sender
// in a given room: their first name (best-effort, derived from their room
// display name), their full room display name, and their username (the
// localpart of their Matrix ID).
type senderNames struct {
	FirstName   string
	DisplayName string
	Username    string
}

// resolveSenderNames looks up the sender's display name in the given room
// via the client's local state store (no network call). Falls back to the
// Matrix ID localpart wherever a display name isn't known.
func resolveSenderNames(ctx context.Context, client *mautrix.Client, roomID id.RoomID, sender id.UserID) senderNames {
	username := sender.Localpart()
	names := senderNames{
		FirstName:   username,
		DisplayName: username,
		Username:    username,
	}

	if client.StateStore == nil {
		return names
	}
	member, err := client.StateStore.TryGetMember(ctx, roomID, sender)
	if err != nil || member == nil || member.Displayname == "" {
		return names
	}

	names.DisplayName = member.Displayname
	if fields := strings.Fields(member.Displayname); len(fields) > 0 {
		names.FirstName = fields[0]
	}
	return names
}

// renderMessage substitutes {firstName}, {displayName}, and {username}
// placeholders in tmpl with the sender's resolved names.
func renderMessage(tmpl string, names senderNames) string {
	replacer := strings.NewReplacer(
		"{firstName}", names.FirstName,
		"{displayName}", names.DisplayName,
		"{username}", names.Username,
	)
	return replacer.Replace(tmpl)
}
