package whatsapp

import (
	"context"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

type labelStoreRepoStub struct {
	domainChatStorage.IChatStorageRepository
	labels       map[string]*domainChatStorage.Label
	associations []*domainChatStorage.LabelChat
}

func (r *labelStoreRepoStub) GetLabel(deviceID, labelID string) (*domainChatStorage.Label, error) {
	return r.labels[deviceID+"/"+labelID], nil
}

func (r *labelStoreRepoStub) UpsertLabel(label *domainChatStorage.Label) error {
	if r.labels == nil {
		r.labels = map[string]*domainChatStorage.Label{}
	}
	r.labels[label.DeviceID+"/"+label.LabelID] = label
	return nil
}

func (r *labelStoreRepoStub) UpsertLabelChat(association *domainChatStorage.LabelChat) error {
	r.associations = append(r.associations, association)
	return nil
}

func labelTestClient(t *testing.T, lid, pn types.JID) (*whatsmeow.Client, string) {
	t.Helper()
	originalLog := log
	log = waLog.Noop
	t.Cleanup(func() { log = originalLog })
	account := types.NewJID("628999999999", types.DefaultUserServer)
	account.Device = 7
	return &whatsmeow.Client{Store: &store.Device{
		ID:   &account,
		LIDs: &chatDisplayNameLIDStore{lid: lid, pn: pn},
	}}, account.ToNonAD().String()
}

func TestHandleLabelEditStoresAndMergesMissingFields(t *testing.T) {
	client, deviceID := labelTestClient(t, types.EmptyJID, types.EmptyJID)
	predefined := int32(2)
	repo := &labelStoreRepoStub{labels: map[string]*domainChatStorage.Label{
		deviceID + "/4": {DeviceID: deviceID, LabelID: "4", Name: "Old", Color: 3, PredefinedID: &predefined},
	}}
	ts := time.UnixMilli(1_760_000_000_000)

	// Only the name changes; color and predefined id must survive.
	handleLabelEdit(context.Background(), &events.LabelEdit{
		LabelID:   "4",
		Timestamp: ts,
		Action:    &waSyncAction.LabelEditAction{Name: proto.String("New")},
	}, repo, client)

	got := repo.labels[deviceID+"/4"]
	if got == nil || got.Name != "New" || got.Color != 3 || got.PredefinedID == nil || *got.PredefinedID != 2 || got.Deleted {
		t.Fatalf("unexpected merged label: %+v", got)
	}
	if !got.UpdatedAt.Equal(ts) {
		t.Fatalf("updated_at = %v, want event timestamp %v", got.UpdatedAt, ts)
	}

	handleLabelEdit(context.Background(), &events.LabelEdit{
		LabelID:   "9",
		Timestamp: ts,
		Action:    &waSyncAction.LabelEditAction{Name: proto.String("Fresh"), Color: proto.Int32(5), Deleted: proto.Bool(true)},
	}, repo, client)
	if got := repo.labels[deviceID+"/9"]; got == nil || got.Name != "Fresh" || got.Color != 5 || !got.Deleted {
		t.Fatalf("unexpected new label: %+v", got)
	}
}

func TestHandleLabelAssociationChatNormalizesLID(t *testing.T) {
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	pn := types.NewJID("628123456789", types.DefaultUserServer)
	client, deviceID := labelTestClient(t, lid, pn)
	repo := &labelStoreRepoStub{}

	handleLabelAssociationChat(context.Background(), &events.LabelAssociationChat{
		JID:       lid,
		LabelID:   "4",
		Timestamp: time.Now(),
		Action:    &waSyncAction.LabelAssociationAction{Labeled: proto.Bool(true)},
	}, repo, client)
	group := types.NewJID("120363000000000042", types.GroupServer)
	handleLabelAssociationChat(context.Background(), &events.LabelAssociationChat{
		JID:       group,
		LabelID:   "4",
		Timestamp: time.Now(),
		Action:    &waSyncAction.LabelAssociationAction{Labeled: proto.Bool(false)},
	}, repo, client)

	if len(repo.associations) != 2 {
		t.Fatalf("stored %d associations, want 2", len(repo.associations))
	}
	first := repo.associations[0]
	if first.DeviceID != deviceID || first.ChatJID != pn.String() || !first.Labeled || first.LabelID != "4" {
		t.Fatalf("unexpected LID association: %+v", first)
	}
	if second := repo.associations[1]; second.ChatJID != group.String() || second.Labeled {
		t.Fatalf("unexpected group association: %+v", second)
	}
}

func TestHandleLabelEventsIgnoreIncompleteInput(t *testing.T) {
	repo := &labelStoreRepoStub{}
	handleLabelEdit(context.Background(), &events.LabelEdit{LabelID: "1", Action: &waSyncAction.LabelEditAction{}}, repo, nil)
	handleLabelAssociationChat(context.Background(), &events.LabelAssociationChat{
		JID: types.NewJID("628123456789", types.DefaultUserServer), LabelID: "1",
	}, repo, &whatsmeow.Client{Store: &store.Device{}})
	if len(repo.labels) != 0 || len(repo.associations) != 0 {
		t.Fatalf("incomplete events must not be stored: %+v %+v", repo.labels, repo.associations)
	}
}
