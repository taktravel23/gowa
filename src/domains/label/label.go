package label

// Request and response structures for WhatsApp Business label operations.
// Labels exist only on WhatsApp Business accounts.

// LabelInfo is one label as returned by the API.
type LabelInfo struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Color        int32  `json:"color"`
	PredefinedID *int32 `json:"predefined_id,omitempty"`
}

type ListLabelsResponse struct {
	Data []LabelInfo `json:"data"`
}

// CreateLabelRequest creates a label. LabelID is optional: when empty, the
// next free numeric id is used. Color defaults to 0 when omitted.
type CreateLabelRequest struct {
	LabelID string `json:"label_id"`
	Name    string `json:"name"`
	Color   *int32 `json:"color"`
}

type CreateLabelResponse struct {
	Status  string    `json:"status"`
	Message string    `json:"message"`
	Label   LabelInfo `json:"label"`
}

// UpdateLabelRequest renames and/or recolors a label; at least one of Name
// and Color is required.
type UpdateLabelRequest struct {
	LabelID string  `json:"label_id" uri:"label_id"`
	Name    *string `json:"name"`
	Color   *int32  `json:"color"`
}

type UpdateLabelResponse struct {
	Status  string    `json:"status"`
	Message string    `json:"message"`
	Label   LabelInfo `json:"label"`
}

type DeleteLabelRequest struct {
	LabelID string `json:"label_id" uri:"label_id"`
}

type DeleteLabelResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	LabelID string `json:"label_id"`
}

type GetLabelChatsRequest struct {
	LabelID string `json:"label_id" uri:"label_id"`
}

type GetLabelChatsResponse struct {
	Label LabelInfo `json:"label"`
	Chats []string  `json:"chats"`
}

// LabelChatRequest adds (Labeled=true) or removes (Labeled=false) a label on a chat.
type LabelChatRequest struct {
	LabelID string `json:"label_id" uri:"label_id"`
	ChatJID string `json:"chat_jid" uri:"chat_jid"`
	Labeled bool   `json:"labeled"`
}

type LabelChatResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	LabelID string `json:"label_id"`
	ChatJID string `json:"chat_jid"`
	Labeled bool   `json:"labeled"`
}
