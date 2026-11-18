package usecase

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainLabel "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/label"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

const labelTestDevice = "628111111111@s.whatsapp.net"

type labelFixture struct {
	repo    domainChatStorage.IChatStorageRepository
	service serviceLabel
	patches []appstate.PatchInfo
	sendErr error
	now     time.Time
}

func newLabelFixture(t *testing.T) *labelFixture {
	t.Helper()
	db, err := sql.Open(sqlite.DriverName, ":memory:")
	require.NoError(t, err)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repo := chatstorage.NewStorageRepository(db)
	require.NoError(t, repo.InitializeSchema())

	f := &labelFixture{repo: repo, now: time.Now()}
	f.service = serviceLabel{
		chatStorageRepo: repo,
		sendAppStateFn: func(_ context.Context, _ *whatsmeow.Client, patch appstate.PatchInfo) error {
			if f.sendErr != nil {
				return f.sendErr
			}
			f.patches = append(f.patches, patch)
			return nil
		},
		validateJIDFn: func(_ *whatsmeow.Client, jid string) (types.JID, error) {
			return types.ParseJID(jid)
		},
		nowFn: func() time.Time {
			f.now = f.now.Add(time.Second)
			return f.now
		},
	}
	return f
}

func (f *labelFixture) seed(t *testing.T, label domainChatStorage.Label) {
	t.Helper()
	label.DeviceID = labelTestDevice
	if label.UpdatedAt.IsZero() {
		label.UpdatedAt = f.now.Add(-time.Hour)
	}
	require.NoError(t, f.repo.UpsertLabel(&label))
}

func (f *labelFixture) lastEdit(t *testing.T) (labelID string, name string, color int32, deleted bool) {
	t.Helper()
	require.NotEmpty(t, f.patches)
	mutation := f.patches[len(f.patches)-1].Mutations[0]
	require.Equal(t, appstate.IndexLabelEdit, mutation.Index[0])
	action := mutation.Value.GetLabelEditAction()
	return mutation.Index[1], action.GetName(), action.GetColor(), action.GetDeleted()
}

func int32Ptr(v int32) *int32 { return &v }

func TestNextLabelID(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want string
	}{
		{"no labels yet", nil, "1"},
		{"numeric max not lexical max", []string{"9", "10", "2"}, "11"},
		{"ignores non numeric ids", []string{"abc", "4", ""}, "5"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextLabelID(tt.ids))
		})
	}
}

func TestCreateLabelPicksNextFreeIDIncludingDeletedAndAssociated(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "3", Name: "a"})
	f.seed(t, domainChatStorage.Label{LabelID: "7", Name: "gone", Deleted: true})
	require.NoError(t, f.repo.UpsertLabelChat(&domainChatStorage.LabelChat{
		DeviceID: labelTestDevice, LabelID: "9", ChatJID: "628333333333@s.whatsapp.net", Labeled: true,
	}))

	resp, err := f.service.CreateLabel(scopeContext(labelTestDevice), domainLabel.CreateLabelRequest{Name: "  VIP  ", Color: int32Ptr(4)})
	require.NoError(t, err)
	assert.Equal(t, domainLabel.LabelInfo{ID: "10", Name: "VIP", Color: 4}, resp.Label)

	id, name, color, deleted := f.lastEdit(t)
	assert.Equal(t, "10", id)
	assert.Equal(t, "VIP", name)
	assert.Equal(t, int32(4), color)
	assert.False(t, deleted)

	stored, err := f.repo.GetLabel(labelTestDevice, "10")
	require.NoError(t, err)
	require.NotNil(t, stored, "created label must be written through")
	assert.Equal(t, "VIP", stored.Name)
}

func TestCreateLabelWithExplicitID(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "5", Name: "taken"})
	f.seed(t, domainChatStorage.Label{LabelID: "6", Name: "old", Deleted: true})
	ctx := scopeContext(labelTestDevice)

	_, err := f.service.CreateLabel(ctx, domainLabel.CreateLabelRequest{LabelID: "5", Name: "dup"})
	require.Error(t, err)
	assert.IsType(t, pkgError.ValidationError(""), err)
	assert.Empty(t, f.patches, "an existing id must be refused before any patch is sent")

	resp, err := f.service.CreateLabel(ctx, domainLabel.CreateLabelRequest{LabelID: "6", Name: "reused"})
	require.NoError(t, err, "a deleted label id may be reused")
	assert.Equal(t, "6", resp.Label.ID)
	assert.Equal(t, int32(0), resp.Label.Color, "color defaults to 0")
}

func TestUpdateLabelFillsMissingFieldFromStore(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "2", Name: "Paid", Color: 6, PredefinedID: int32Ptr(4)})
	ctx := scopeContext(labelTestDevice)

	newName := "Paid in full"
	resp, err := f.service.UpdateLabel(ctx, domainLabel.UpdateLabelRequest{LabelID: "2", Name: &newName})
	require.NoError(t, err)
	id, name, color, deleted := f.lastEdit(t)
	assert.Equal(t, "2", id)
	assert.Equal(t, "Paid in full", name)
	assert.Equal(t, int32(6), color, "omitted color must come from the store")
	assert.False(t, deleted)
	require.NotNil(t, resp.Label.PredefinedID)
	assert.Equal(t, int32(4), *resp.Label.PredefinedID)

	resp, err = f.service.UpdateLabel(ctx, domainLabel.UpdateLabelRequest{LabelID: "2", Color: int32Ptr(1)})
	require.NoError(t, err)
	_, name, color, _ = f.lastEdit(t)
	assert.Equal(t, "Paid in full", name, "omitted name must come from the (written-through) store")
	assert.Equal(t, int32(1), color)
	assert.Equal(t, int32(1), resp.Label.Color)
}

func TestLabelNotFound(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "4", Name: "gone", Deleted: true})
	ctx := scopeContext(labelTestDevice)
	name := "x"

	for _, labelID := range []string{"99", "4"} {
		_, err := f.service.UpdateLabel(ctx, domainLabel.UpdateLabelRequest{LabelID: labelID, Name: &name})
		assert.ErrorIs(t, err, pkgError.ErrLabelNotFound, "update %s", labelID)
		_, err = f.service.DeleteLabel(ctx, domainLabel.DeleteLabelRequest{LabelID: labelID})
		assert.ErrorIs(t, err, pkgError.ErrLabelNotFound, "delete %s", labelID)
		_, err = f.service.GetLabelChats(ctx, domainLabel.GetLabelChatsRequest{LabelID: labelID})
		assert.ErrorIs(t, err, pkgError.ErrLabelNotFound, "chats %s", labelID)
		_, err = f.service.SetChatLabel(ctx, domainLabel.LabelChatRequest{LabelID: labelID, ChatJID: "628333333333@s.whatsapp.net", Labeled: true})
		assert.ErrorIs(t, err, pkgError.ErrLabelNotFound, "add chat %s", labelID)
	}
	assert.Empty(t, f.patches, "no patch may be sent for an unknown label")

	_, err := f.service.SetChatLabel(ctx, domainLabel.LabelChatRequest{LabelID: "99", ChatJID: "628333333333@s.whatsapp.net", Labeled: false})
	assert.NoError(t, err, "removing an unknown label from a chat stays idempotent")
}

func TestDeleteLabelHidesItFromList(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "1", Name: "Keep", Color: 2})
	f.seed(t, domainChatStorage.Label{LabelID: "2", Name: "Drop", Color: 3})
	ctx := scopeContext(labelTestDevice)

	_, err := f.service.DeleteLabel(ctx, domainLabel.DeleteLabelRequest{LabelID: "2"})
	require.NoError(t, err)
	id, name, color, deleted := f.lastEdit(t)
	assert.Equal(t, "2", id)
	assert.Equal(t, "Drop", name)
	assert.Equal(t, int32(3), color)
	assert.True(t, deleted)

	list, err := f.service.ListLabels(ctx)
	require.NoError(t, err)
	assert.Equal(t, []domainLabel.LabelInfo{{ID: "1", Name: "Keep", Color: 2}}, list.Data)
}

func TestSetChatLabelNormalizesLIDAndListsChats(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "3", Name: "Lead"})
	lid := types.NewJID("123456789012345", types.HiddenUserServer)
	pn := types.NewJID("628333333333", types.DefaultUserServer)
	f.service.normalizeJIDFn = func(_ context.Context, jid types.JID, _ *whatsmeow.Client) types.JID {
		if jid == lid {
			return pn
		}
		return jid
	}
	ctx := scopeContext(labelTestDevice)

	resp, err := f.service.SetChatLabel(ctx, domainLabel.LabelChatRequest{LabelID: "3", ChatJID: lid.String(), Labeled: true})
	require.NoError(t, err)
	assert.Equal(t, pn.String(), resp.ChatJID)
	mutation := f.patches[0].Mutations[0]
	assert.Equal(t, []string{appstate.IndexLabelAssociationChat, "3", lid.String()}, mutation.Index)
	assert.True(t, mutation.Value.GetLabelAssociationAction().GetLabeled())

	chats, err := f.service.GetLabelChats(ctx, domainLabel.GetLabelChatsRequest{LabelID: "3"})
	require.NoError(t, err)
	assert.Equal(t, []string{pn.String()}, chats.Chats)
	assert.Equal(t, "Lead", chats.Label.Name)

	_, err = f.service.SetChatLabel(ctx, domainLabel.LabelChatRequest{LabelID: "3", ChatJID: pn.String(), Labeled: false})
	require.NoError(t, err)
	chats, err = f.service.GetLabelChats(ctx, domainLabel.GetLabelChatsRequest{LabelID: "3"})
	require.NoError(t, err)
	assert.Empty(t, chats.Chats)
}

func TestLabelWritesStoreNothingWhenSendFails(t *testing.T) {
	f := newLabelFixture(t)
	f.sendErr = errors.New("app state rejected")
	ctx := scopeContext(labelTestDevice)

	_, err := f.service.CreateLabel(ctx, domainLabel.CreateLabelRequest{Name: "VIP"})
	require.ErrorIs(t, err, f.sendErr)
	list, err := f.service.ListLabels(ctx)
	require.NoError(t, err)
	assert.Empty(t, list.Data)
}

func TestLabelWritesRequireClientAndReadsRequireDevice(t *testing.T) {
	f := newLabelFixture(t)
	noClient := whatsapp.ContextWithDevice(context.Background(), whatsapp.NewDeviceInstance(labelTestDevice, nil, nil))

	_, err := f.service.CreateLabel(noClient, domainLabel.CreateLabelRequest{Name: "VIP"})
	assert.Equal(t, pkgError.ErrWaCLI, err)
	assert.Empty(t, f.patches)

	_, err = f.service.ListLabels(context.Background())
	assert.Error(t, err, "listing without a device must not read every device's labels")
}

func TestListLabelsIsDeviceScoped(t *testing.T) {
	f := newLabelFixture(t)
	f.seed(t, domainChatStorage.Label{LabelID: "1", Name: "Mine"})
	require.NoError(t, f.repo.UpsertLabel(&domainChatStorage.Label{DeviceID: "628222222222@s.whatsapp.net", LabelID: "2", Name: "Theirs"}))

	list, err := f.service.ListLabels(scopeContext(labelTestDevice))
	require.NoError(t, err)
	assert.Equal(t, []domainLabel.LabelInfo{{ID: "1", Name: "Mine"}}, list.Data)
}
