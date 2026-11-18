package usecase

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainLabel "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/label"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/validations"
	"github.com/sirupsen/logrus"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

// serviceLabel manages WhatsApp Business labels. whatsmeow keeps no label
// list, so reads come from chat storage, which app-state events fill. Each
// successful patch is also written through, since GOWA cannot rely on its own
// patch coming back as an event before the caller reads again.
type serviceLabel struct {
	chatStorageRepo domainChatStorage.IChatStorageRepository

	// Seams for tests; nil uses the real whatsmeow/GOWA implementations.
	validateJIDFn  func(client *whatsmeow.Client, jid string) (types.JID, error)
	normalizeJIDFn func(ctx context.Context, jid types.JID, client *whatsmeow.Client) types.JID
	sendAppStateFn func(ctx context.Context, client *whatsmeow.Client, patch appstate.PatchInfo) error
	nowFn          func() time.Time
}

func NewLabelService(chatStorageRepo domainChatStorage.IChatStorageRepository) domainLabel.ILabelUsecase {
	return &serviceLabel{chatStorageRepo: chatStorageRepo}
}

func (service serviceLabel) validateJID(client *whatsmeow.Client, jid string) (types.JID, error) {
	if service.validateJIDFn != nil {
		return service.validateJIDFn(client, jid)
	}
	return utils.ValidateJidWithLogin(client, jid)
}

func (service serviceLabel) normalizeJID(ctx context.Context, jid types.JID, client *whatsmeow.Client) types.JID {
	if service.normalizeJIDFn != nil {
		return service.normalizeJIDFn(ctx, jid, client)
	}
	return whatsapp.NormalizeJIDFromLID(ctx, jid, client)
}

func (service serviceLabel) sendAppState(ctx context.Context, client *whatsmeow.Client, patch appstate.PatchInfo) error {
	if service.sendAppStateFn != nil {
		return service.sendAppStateFn(ctx, client, patch)
	}
	return client.SendAppState(ctx, patch)
}

func (service serviceLabel) now() time.Time {
	if service.nowFn != nil {
		return service.nowFn()
	}
	return time.Now()
}

// labelDeviceID resolves the storage identity of the calling device.
func labelDeviceID(ctx context.Context) (string, error) {
	deviceID := storageDeviceID(ctx)
	if deviceID == "" {
		return "", fmt.Errorf("device identification required")
	}
	return deviceID, nil
}

// labelClientAndDevice resolves the client and storage identity for a write.
func labelClientAndDevice(ctx context.Context) (*whatsmeow.Client, string, error) {
	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return nil, "", pkgError.ErrWaCLI
	}
	deviceID, err := labelDeviceID(ctx)
	if err != nil {
		return nil, "", err
	}
	return client, deviceID, nil
}

// activeLabel returns a stored, non-deleted label or ErrLabelNotFound.
func (service serviceLabel) activeLabel(deviceID, labelID string) (*domainChatStorage.Label, error) {
	label, err := service.chatStorageRepo.GetLabel(deviceID, labelID)
	if err != nil {
		return nil, err
	}
	if label == nil || label.Deleted {
		return nil, pkgError.ErrLabelNotFound
	}
	return label, nil
}

func toLabelInfo(label *domainChatStorage.Label) domainLabel.LabelInfo {
	return domainLabel.LabelInfo{
		ID:           label.LabelID,
		Name:         label.Name,
		Color:        label.Color,
		PredefinedID: label.PredefinedID,
	}
}

// nextLabelID returns one more than the highest numeric id ever seen, deleted
// labels included, so a fresh label never reuses an id WhatsApp still knows.
func nextLabelID(knownIDs []string) string {
	var highest uint64
	for _, id := range knownIDs {
		value, err := strconv.ParseUint(strings.TrimSpace(id), 10, 63)
		if err == nil && value > highest {
			highest = value
		}
	}
	return strconv.FormatUint(highest+1, 10)
}

func (service serviceLabel) ListLabels(ctx context.Context) (response domainLabel.ListLabelsResponse, err error) {
	deviceID, err := labelDeviceID(ctx)
	if err != nil {
		return response, err
	}

	labels, err := service.chatStorageRepo.ListLabels(deviceID, false)
	if err != nil {
		return response, err
	}

	response.Data = make([]domainLabel.LabelInfo, 0, len(labels))
	for _, label := range labels {
		response.Data = append(response.Data, toLabelInfo(label))
	}
	return response, nil
}

func (service serviceLabel) CreateLabel(ctx context.Context, request domainLabel.CreateLabelRequest) (response domainLabel.CreateLabelResponse, err error) {
	if err = validations.ValidateCreateLabel(ctx, &request); err != nil {
		return response, err
	}

	client, deviceID, err := labelClientAndDevice(ctx)
	if err != nil {
		return response, err
	}

	labelID := request.LabelID
	if labelID != "" {
		existing, err := service.chatStorageRepo.GetLabel(deviceID, labelID)
		if err != nil {
			return response, err
		}
		if existing != nil && !existing.Deleted {
			return response, pkgError.ValidationError(fmt.Sprintf("label %s already exists", labelID))
		}
	} else {
		knownIDs, err := service.chatStorageRepo.ListKnownLabelIDs(deviceID)
		if err != nil {
			return response, err
		}
		labelID = nextLabelID(knownIDs)
	}

	name := strings.TrimSpace(request.Name)
	var color int32
	if request.Color != nil {
		color = *request.Color
	}

	if err = service.sendAppState(ctx, client, appstate.BuildLabelEdit(labelID, name, color, false)); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": labelID}).Error("Failed to send create label app state")
		return response, err
	}

	label := &domainChatStorage.Label{DeviceID: deviceID, LabelID: labelID, Name: name, Color: color, UpdatedAt: service.now()}
	if err := service.chatStorageRepo.UpsertLabel(label); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": labelID}).Warn("Label created on WhatsApp but not stored locally")
	}

	response.Status = "success"
	response.Message = "Label created successfully"
	response.Label = toLabelInfo(label)
	return response, nil
}

func (service serviceLabel) UpdateLabel(ctx context.Context, request domainLabel.UpdateLabelRequest) (response domainLabel.UpdateLabelResponse, err error) {
	if err = validations.ValidateUpdateLabel(ctx, &request); err != nil {
		return response, err
	}

	client, deviceID, err := labelClientAndDevice(ctx)
	if err != nil {
		return response, err
	}

	existing, err := service.activeLabel(deviceID, request.LabelID)
	if err != nil {
		return response, err
	}

	// BuildLabelEdit always sends both fields; keep the stored value for the omitted one.
	name := existing.Name
	if request.Name != nil {
		name = strings.TrimSpace(*request.Name)
	}
	color := existing.Color
	if request.Color != nil {
		color = *request.Color
	}

	if err = service.sendAppState(ctx, client, appstate.BuildLabelEdit(request.LabelID, name, color, false)); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": request.LabelID}).Error("Failed to send update label app state")
		return response, err
	}

	label := &domainChatStorage.Label{
		DeviceID:     deviceID,
		LabelID:      request.LabelID,
		Name:         name,
		Color:        color,
		PredefinedID: existing.PredefinedID,
		UpdatedAt:    service.now(),
	}
	if err := service.chatStorageRepo.UpsertLabel(label); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": request.LabelID}).Warn("Label updated on WhatsApp but not stored locally")
	}

	response.Status = "success"
	response.Message = "Label updated successfully"
	response.Label = toLabelInfo(label)
	return response, nil
}

func (service serviceLabel) DeleteLabel(ctx context.Context, request domainLabel.DeleteLabelRequest) (response domainLabel.DeleteLabelResponse, err error) {
	if err = validations.ValidateDeleteLabel(ctx, &request); err != nil {
		return response, err
	}

	client, deviceID, err := labelClientAndDevice(ctx)
	if err != nil {
		return response, err
	}

	existing, err := service.activeLabel(deviceID, request.LabelID)
	if err != nil {
		return response, err
	}

	if err = service.sendAppState(ctx, client, appstate.BuildLabelEdit(request.LabelID, existing.Name, existing.Color, true)); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": request.LabelID}).Error("Failed to send delete label app state")
		return response, err
	}

	deleted := *existing
	deleted.Deleted = true
	deleted.UpdatedAt = service.now()
	if err := service.chatStorageRepo.UpsertLabel(&deleted); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": request.LabelID}).Warn("Label deleted on WhatsApp but not stored locally")
	}

	response.Status = "success"
	response.Message = "Label deleted successfully"
	response.LabelID = request.LabelID
	return response, nil
}

func (service serviceLabel) GetLabelChats(ctx context.Context, request domainLabel.GetLabelChatsRequest) (response domainLabel.GetLabelChatsResponse, err error) {
	if err = validations.ValidateGetLabelChats(ctx, &request); err != nil {
		return response, err
	}

	deviceID, err := labelDeviceID(ctx)
	if err != nil {
		return response, err
	}

	label, err := service.activeLabel(deviceID, request.LabelID)
	if err != nil {
		return response, err
	}

	chats, err := service.chatStorageRepo.ListLabelChatJIDs(deviceID, request.LabelID)
	if err != nil {
		return response, err
	}

	response.Label = toLabelInfo(label)
	response.Chats = chats
	return response, nil
}

// SetChatLabel adds or removes a label on a chat. Adding requires a known,
// non-deleted label; removing does not, so it stays idempotent.
func (service serviceLabel) SetChatLabel(ctx context.Context, request domainLabel.LabelChatRequest) (response domainLabel.LabelChatResponse, err error) {
	if err = validations.ValidateLabelChat(ctx, &request); err != nil {
		return response, err
	}

	client, deviceID, err := labelClientAndDevice(ctx)
	if err != nil {
		return response, err
	}

	if request.Labeled {
		if _, err = service.activeLabel(deviceID, request.LabelID); err != nil {
			return response, err
		}
	}

	targetJID, err := service.validateJID(client, request.ChatJID)
	if err != nil {
		return response, err
	}

	if err = service.sendAppState(ctx, client, appstate.BuildLabelChat(targetJID, request.LabelID, request.Labeled)); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"label_id": request.LabelID,
			"chat_jid": request.ChatJID,
			"labeled":  request.Labeled,
		}).Error("Failed to send label chat app state")
		return response, err
	}

	// Store under the phone JID, as label association events do.
	chatJID := service.normalizeJID(ctx, targetJID.ToNonAD(), client).ToNonAD().String()
	if err := service.chatStorageRepo.UpsertLabelChat(&domainChatStorage.LabelChat{
		DeviceID:  deviceID,
		LabelID:   request.LabelID,
		ChatJID:   chatJID,
		Labeled:   request.Labeled,
		UpdatedAt: service.now(),
	}); err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{"label_id": request.LabelID, "chat_jid": chatJID}).Warn("Chat label changed on WhatsApp but not stored locally")
	}

	response.Status = "success"
	response.LabelID = request.LabelID
	response.ChatJID = chatJID
	response.Labeled = request.Labeled
	if request.Labeled {
		response.Message = "Label added to chat successfully"
	} else {
		response.Message = "Label removed from chat successfully"
	}
	return response, nil
}
