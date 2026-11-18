package chatstorage

import "time"

// Label is one WhatsApp Business label as last seen by a device. whatsmeow
// keeps no label list, so GOWA persists what app-state sync and its own
// patches report. Rows are scoped by DeviceID (the logged-in account JID) and
// a deleted label keeps its row so its numeric id is never handed out again.
type Label struct {
	DeviceID     string
	LabelID      string
	Name         string
	Color        int32
	PredefinedID *int32
	Deleted      bool
	UpdatedAt    time.Time
}

// LabelChat is the latest known association state between a label and a chat.
// Labeled=false rows are kept so an older event cannot re-apply a removed label.
type LabelChat struct {
	DeviceID  string
	LabelID   string
	ChatJID   string
	Labeled   bool
	UpdatedAt time.Time
}
