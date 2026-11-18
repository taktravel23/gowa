package chatstorage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
)

// UpsertLabel stores a device's label. A row whose stored updated_at is newer
// than the incoming one is kept, so a late full-sync event cannot overwrite a
// more recent edit. Storing a deleted label also clears its chat associations.
func (r *SQLiteRepository) UpsertLabel(label *domainChatStorage.Label) error {
	if label == nil {
		return fmt.Errorf("label is required")
	}
	if strings.TrimSpace(label.DeviceID) == "" || strings.TrimSpace(label.LabelID) == "" {
		return fmt.Errorf("label requires device_id and label_id")
	}
	if label.UpdatedAt.IsZero() {
		label.UpdatedAt = time.Now()
	}

	var predefinedID sql.NullInt32
	if label.PredefinedID != nil {
		predefinedID = sql.NullInt32{Int32: *label.PredefinedID, Valid: true}
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingUpdatedAt time.Time
	err = tx.QueryRow(`SELECT updated_at FROM labels WHERE device_id = ? AND label_id = ?`,
		label.DeviceID, label.LabelID).Scan(&existingUpdatedAt)
	switch {
	case err == nil:
		if label.UpdatedAt.Before(existingUpdatedAt) {
			return tx.Commit()
		}
		if _, err = tx.Exec(`
			UPDATE labels SET name = ?, color = ?, predefined_id = ?, deleted = ?, updated_at = ?
			WHERE device_id = ? AND label_id = ?
		`, label.Name, label.Color, predefinedID, label.Deleted, label.UpdatedAt,
			label.DeviceID, label.LabelID); err != nil {
			return err
		}
	case err == sql.ErrNoRows:
		if _, err = tx.Exec(`
			INSERT INTO labels (device_id, label_id, name, color, predefined_id, deleted, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, label.DeviceID, label.LabelID, label.Name, label.Color, predefinedID, label.Deleted, label.UpdatedAt); err != nil {
			return err
		}
	default:
		return err
	}

	if label.Deleted {
		if _, err = tx.Exec(`DELETE FROM label_chats WHERE device_id = ? AND label_id = ?`,
			label.DeviceID, label.LabelID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetLabel returns one stored label, deleted or not, or nil when unknown.
func (r *SQLiteRepository) GetLabel(deviceID, labelID string) (*domainChatStorage.Label, error) {
	label, err := scanLabel(r.db.QueryRow(`
		SELECT device_id, label_id, name, color, predefined_id, deleted, updated_at
		FROM labels WHERE device_id = ? AND label_id = ?
	`, deviceID, labelID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return label, err
}

// ListLabels returns the device's labels ordered by numeric id.
func (r *SQLiteRepository) ListLabels(deviceID string, includeDeleted bool) ([]*domainChatStorage.Label, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("device id is required")
	}
	query := `
		SELECT device_id, label_id, name, color, predefined_id, deleted, updated_at
		FROM labels WHERE device_id = ?`
	if !includeDeleted {
		query += ` AND deleted = FALSE`
	}
	query += ` ORDER BY CAST(label_id AS INTEGER), label_id`

	rows, err := r.db.Query(query, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	labels := make([]*domainChatStorage.Label, 0)
	for rows.Next() {
		label, err := scanLabel(rows)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, rows.Err()
}

// ListKnownLabelIDs returns every label id seen for the device, from labels
// and from chat associations whose label edit has not arrived yet.
func (r *SQLiteRepository) ListKnownLabelIDs(deviceID string) ([]string, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("device id is required")
	}
	rows, err := r.db.Query(`
		SELECT label_id FROM labels WHERE device_id = ?
		UNION
		SELECT label_id FROM label_chats WHERE device_id = ?
	`, deviceID, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// UpsertLabelChat stores the latest label state of a chat, ignoring an
// association older than the stored one.
func (r *SQLiteRepository) UpsertLabelChat(association *domainChatStorage.LabelChat) error {
	if association == nil {
		return fmt.Errorf("label association is required")
	}
	if strings.TrimSpace(association.DeviceID) == "" || strings.TrimSpace(association.LabelID) == "" || strings.TrimSpace(association.ChatJID) == "" {
		return fmt.Errorf("label association requires device_id, label_id, and chat_jid")
	}
	if association.UpdatedAt.IsZero() {
		association.UpdatedAt = time.Now()
	}

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var existingUpdatedAt time.Time
	err = tx.QueryRow(`
		SELECT updated_at FROM label_chats WHERE device_id = ? AND label_id = ? AND chat_jid = ?
	`, association.DeviceID, association.LabelID, association.ChatJID).Scan(&existingUpdatedAt)
	switch {
	case err == nil:
		if association.UpdatedAt.Before(existingUpdatedAt) {
			return tx.Commit()
		}
		_, err = tx.Exec(`
			UPDATE label_chats SET labeled = ?, updated_at = ?
			WHERE device_id = ? AND label_id = ? AND chat_jid = ?
		`, association.Labeled, association.UpdatedAt, association.DeviceID, association.LabelID, association.ChatJID)
	case err == sql.ErrNoRows:
		_, err = tx.Exec(`
			INSERT INTO label_chats (device_id, label_id, chat_jid, labeled, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, association.DeviceID, association.LabelID, association.ChatJID, association.Labeled, association.UpdatedAt)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ListLabelChatJIDs returns the chats currently carrying a label.
func (r *SQLiteRepository) ListLabelChatJIDs(deviceID, labelID string) ([]string, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("device id is required")
	}
	rows, err := r.db.Query(`
		SELECT chat_jid FROM label_chats
		WHERE device_id = ? AND label_id = ? AND labeled = TRUE
		ORDER BY chat_jid
	`, deviceID, labelID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	jids := make([]string, 0)
	for rows.Next() {
		var jid string
		if err := rows.Scan(&jid); err != nil {
			return nil, err
		}
		jids = append(jids, jid)
	}
	return jids, rows.Err()
}

func scanLabel(scanner interface{ Scan(...any) error }) (*domainChatStorage.Label, error) {
	var label domainChatStorage.Label
	var predefinedID sql.NullInt32
	if err := scanner.Scan(&label.DeviceID, &label.LabelID, &label.Name, &label.Color, &predefinedID,
		&label.Deleted, &label.UpdatedAt); err != nil {
		return nil, err
	}
	if predefinedID.Valid {
		value := predefinedID.Int32
		label.PredefinedID = &value
	}
	return &label, nil
}
