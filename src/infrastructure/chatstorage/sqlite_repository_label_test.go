package chatstorage

import (
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	labelDeviceA = "628111111111@s.whatsapp.net"
	labelDeviceB = "628222222222@s.whatsapp.net"
)

func TestLabelRoundTripOrderingAndIsolation(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	predefined := int32(3)
	now := time.Now()

	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "10", Name: "VIP", Color: 4, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "2", Name: "Paid", Color: 1, PredefinedID: &predefined, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "5", Name: "Gone", Deleted: true, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceB, LabelID: "7", Name: "Other device", UpdatedAt: now}))

	active, err := repo.ListLabels(labelDeviceA, false)
	require.NoError(t, err)
	require.Len(t, active, 2)
	assert.Equal(t, "2", active[0].LabelID, "labels must be ordered numerically, not lexically")
	assert.Equal(t, "10", active[1].LabelID)
	require.NotNil(t, active[0].PredefinedID)
	assert.Equal(t, int32(3), *active[0].PredefinedID)
	assert.Nil(t, active[1].PredefinedID)
	assert.Equal(t, int32(4), active[1].Color)

	all, err := repo.ListLabels(labelDeviceA, true)
	require.NoError(t, err)
	assert.Len(t, all, 3)

	got, err := repo.GetLabel(labelDeviceA, "5")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Deleted)

	missing, err := repo.GetLabel(labelDeviceA, "7")
	require.NoError(t, err)
	assert.Nil(t, missing, "label leaked across devices")
}

func TestUpsertLabelIgnoresOlderUpdate(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	now := time.Now()

	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "1", Name: "New", Color: 2, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "1", Name: "Stale", Color: 9, UpdatedAt: now.Add(-time.Hour)}))

	got, err := repo.GetLabel(labelDeviceA, "1")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "New", got.Name)
	assert.Equal(t, int32(2), got.Color)

	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "1", Name: "Renamed", Color: 3, UpdatedAt: now.Add(time.Minute)}))
	got, err = repo.GetLabel(labelDeviceA, "1")
	require.NoError(t, err)
	assert.Equal(t, "Renamed", got.Name)
}

func TestLabelChatAssociations(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	now := time.Now()
	chatA := "628333333333@s.whatsapp.net"
	chatB := "120363000000000042@g.us"

	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "1", Name: "Lead", UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: labelDeviceA, LabelID: "1", ChatJID: chatB, Labeled: true, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: labelDeviceA, LabelID: "1", ChatJID: chatA, Labeled: true, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: labelDeviceB, LabelID: "1", ChatJID: chatA, Labeled: true, UpdatedAt: now}))

	jids, err := repo.ListLabelChatJIDs(labelDeviceA, "1")
	require.NoError(t, err)
	assert.Equal(t, []string{chatB, chatA}, jids)

	// Removing hides the chat; a stale "labeled" event must not bring it back.
	require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: labelDeviceA, LabelID: "1", ChatJID: chatA, Labeled: false, UpdatedAt: now.Add(time.Minute)}))
	require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: labelDeviceA, LabelID: "1", ChatJID: chatA, Labeled: true, UpdatedAt: now.Add(-time.Minute)}))
	jids, err = repo.ListLabelChatJIDs(labelDeviceA, "1")
	require.NoError(t, err)
	assert.Equal(t, []string{chatB}, jids)

	// Deleting the label clears its associations.
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "1", Name: "Lead", Deleted: true, UpdatedAt: now.Add(2 * time.Minute)}))
	jids, err = repo.ListLabelChatJIDs(labelDeviceA, "1")
	require.NoError(t, err)
	assert.Empty(t, jids)

	other, err := repo.ListLabelChatJIDs(labelDeviceB, "1")
	require.NoError(t, err)
	assert.Equal(t, []string{chatA}, other, "deleting device A's label must not touch device B")
}

func TestListKnownLabelIDsIncludesDeletedAndAssociationOnly(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	now := time.Now()

	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "3", Name: "a", UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceA, LabelID: "8", Name: "b", Deleted: true, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: labelDeviceA, LabelID: "12", ChatJID: "628333333333@s.whatsapp.net", Labeled: true, UpdatedAt: now}))
	require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: labelDeviceB, LabelID: "99", Name: "c", UpdatedAt: now}))

	ids, err := repo.ListKnownLabelIDs(labelDeviceA)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"3", "8", "12"}, ids)
}

func TestDeviceCleanupRemovesLabels(t *testing.T) {
	repo := newTestSQLiteRepository(t)
	now := time.Now()
	for _, device := range []string{labelDeviceA, labelDeviceB} {
		require.NoError(t, repo.UpsertLabel(&domainChatStorage.Label{DeviceID: device, LabelID: "1", Name: "x", UpdatedAt: now}))
		require.NoError(t, repo.UpsertLabelChat(&domainChatStorage.LabelChat{DeviceID: device, LabelID: "1", ChatJID: "628333333333@s.whatsapp.net", Labeled: true, UpdatedAt: now}))
	}

	require.NoError(t, repo.DeleteDeviceData(labelDeviceA))
	ids, err := repo.ListKnownLabelIDs(labelDeviceA)
	require.NoError(t, err)
	assert.Empty(t, ids)
	ids, err = repo.ListKnownLabelIDs(labelDeviceB)
	require.NoError(t, err)
	assert.Equal(t, []string{"1"}, ids)

	require.NoError(t, repo.TruncateAllChats())
	ids, err = repo.ListKnownLabelIDs(labelDeviceB)
	require.NoError(t, err)
	assert.Empty(t, ids)
}
