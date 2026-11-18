package whatsapp

import (
	"context"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

const (
	eventTypeLabelEdit        = "label.edit"
	eventTypeLabelAssociation = "label.association"
)

func isLabelAppState(evt *events.AppState) bool {
	if evt == nil || len(evt.Index) == 0 {
		return false
	}

	// Note: IndexLabelAssociationMessage (per-message label) is intentionally excluded.
	// Only chat-level associations and label edits are forwarded.
	return evt.Index[0] == appstate.IndexLabelEdit || evt.Index[0] == appstate.IndexLabelAssociationChat
}

func forwardLabelAppStateToWebhook(ctx context.Context, evt *events.AppState, deviceID string, client *whatsmeow.Client) error {
	eventName, payload := buildLabelAppStatePayload(ctx, evt, client)
	if eventName == "" {
		return nil
	}

	body := map[string]any{
		"event":     eventName,
		"timestamp": labelAppStateTimestamp(evt),
		"payload":   payload,
	}
	if deviceID != "" {
		body["device_id"] = deviceID
	}

	return forwardPayloadToConfiguredWebhooks(ctx, body, eventName)
}

func buildLabelAppStatePayload(ctx context.Context, evt *events.AppState, client *whatsmeow.Client) (string, map[string]any) {
	if evt == nil || evt.SyncActionValue == nil || len(evt.Index) == 0 {
		return "", nil
	}

	switch evt.Index[0] {
	case appstate.IndexLabelEdit:
		if len(evt.Index) < 2 || evt.LabelEditAction == nil {
			return "", nil
		}

		payload := map[string]any{"label_id": evt.Index[1]}
		addLabelEditActionFields(payload, evt.LabelEditAction)

		return eventTypeLabelEdit, payload
	case appstate.IndexLabelAssociationChat:
		if len(evt.Index) < 3 || evt.LabelAssociationAction == nil {
			return "", nil
		}

		payload := map[string]any{
			"label_id": evt.Index[1],
			"labeled":  evt.LabelAssociationAction.GetLabeled(),
		}
		addLabelChatFields(ctx, client, payload, evt.Index[2])

		return eventTypeLabelAssociation, payload
	default:
		return "", nil
	}
}

func addLabelEditActionFields(payload map[string]any, action *waSyncAction.LabelEditAction) {
	if action.Name != nil {
		payload["name"] = action.GetName()
	}
	if action.Color != nil {
		payload["color"] = action.GetColor()
	}
	if action.PredefinedID != nil {
		payload["predefined_id"] = action.GetPredefinedID()
	}
	if action.Deleted != nil {
		payload["deleted"] = action.GetDeleted()
	}
	if action.OrderIndex != nil {
		payload["order_index"] = action.GetOrderIndex()
	}
	if action.IsActive != nil {
		payload["is_active"] = action.GetIsActive()
	}
	if action.Type != nil {
		payload["type"] = action.GetType().String()
	}
	if action.IsImmutable != nil {
		payload["is_immutable"] = action.GetIsImmutable()
	}
	if action.MuteEndTimeMS != nil {
		payload["mute_end_time_ms"] = action.GetMuteEndTimeMS()
	}
}

func addLabelChatFields(ctx context.Context, client *whatsmeow.Client, payload map[string]any, rawJID string) {
	jid, err := types.ParseJID(rawJID)
	if err != nil {
		payload["chat_id"] = rawJID

		return
	}

	chatJID := jid.ToNonAD()
	if chatJID.Server == "lid" {
		payload["chat_lid"] = chatJID.String()
		if client != nil {
			chatJID = NormalizeJIDFromLID(ctx, chatJID, client).ToNonAD()
		}
	}

	payload["chat_id"] = chatJID.String()
}

func labelAppStateTimestamp(evt *events.AppState) string {
	if ts := evt.GetTimestamp(); ts > 0 {
		return time.UnixMilli(ts).UTC().Format(time.RFC3339)
	}

	return time.Now().UTC().Format(time.RFC3339)
}

// labelStorageDeviceID is the chat-storage identity of the client's account.
func labelStorageDeviceID(client *whatsmeow.Client) string {
	if client == nil || client.Store == nil || client.Store.ID == nil {
		return ""
	}
	return client.Store.ID.ToNonAD().String()
}

// handleLabelEdit persists a label edit so the REST API can list labels.
// Fields WhatsApp leaves out keep their stored value.
func handleLabelEdit(ctx context.Context, evt *events.LabelEdit, chatStorageRepo domainChatStorage.IChatStorageRepository, client *whatsmeow.Client) {
	deviceID := labelStorageDeviceID(client)
	if evt == nil || evt.Action == nil || evt.LabelID == "" || chatStorageRepo == nil || deviceID == "" {
		return
	}
	logFields := logrus.Fields{"device_id": deviceID, "label_id": evt.LabelID}

	existing, err := chatStorageRepo.GetLabel(deviceID, evt.LabelID)
	if err != nil {
		logrus.WithError(err).WithFields(logFields).Error("Failed to read stored label")
		return
	}
	label := mergeLabelEdit(deviceID, evt, existing)
	if err := chatStorageRepo.UpsertLabel(label); err != nil {
		logrus.WithError(err).WithFields(logFields).Error("Failed to store label edit")
	}
}

func mergeLabelEdit(deviceID string, evt *events.LabelEdit, existing *domainChatStorage.Label) *domainChatStorage.Label {
	label := &domainChatStorage.Label{DeviceID: deviceID, LabelID: evt.LabelID, UpdatedAt: evt.Timestamp}
	if existing != nil {
		label.Name = existing.Name
		label.Color = existing.Color
		label.PredefinedID = existing.PredefinedID
		label.Deleted = existing.Deleted
	}
	action := evt.Action
	if action.Name != nil {
		label.Name = action.GetName()
	}
	if action.Color != nil {
		label.Color = action.GetColor()
	}
	if action.PredefinedID != nil {
		predefinedID := action.GetPredefinedID()
		label.PredefinedID = &predefinedID
	}
	if action.Deleted != nil {
		label.Deleted = action.GetDeleted()
	}
	if label.UpdatedAt.IsZero() || label.UpdatedAt.Unix() <= 0 {
		label.UpdatedAt = time.Now()
	}
	return label
}

// handleLabelAssociationChat persists a chat (un)labeling. LID chats are
// stored under their phone JID, as forwarded to webhooks.
func handleLabelAssociationChat(ctx context.Context, evt *events.LabelAssociationChat, chatStorageRepo domainChatStorage.IChatStorageRepository, client *whatsmeow.Client) {
	deviceID := labelStorageDeviceID(client)
	if evt == nil || evt.Action == nil || evt.LabelID == "" || evt.JID.IsEmpty() || chatStorageRepo == nil || deviceID == "" {
		return
	}

	chatJID := NormalizeJIDFromLID(ctx, evt.JID.ToNonAD(), client).ToNonAD().String()
	updatedAt := evt.Timestamp
	if updatedAt.IsZero() || updatedAt.Unix() <= 0 {
		updatedAt = time.Now()
	}
	if err := chatStorageRepo.UpsertLabelChat(&domainChatStorage.LabelChat{
		DeviceID:  deviceID,
		LabelID:   evt.LabelID,
		ChatJID:   chatJID,
		Labeled:   evt.Action.GetLabeled(),
		UpdatedAt: updatedAt,
	}); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"device_id": deviceID, "label_id": evt.LabelID, "chat_jid": chatJID,
		}).Error("Failed to store label association")
	}
}
