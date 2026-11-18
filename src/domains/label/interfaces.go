package label

import (
	"context"
)

// ILabelUsecase defines WhatsApp Business label operations for one device.
type ILabelUsecase interface {
	ListLabels(ctx context.Context) (response ListLabelsResponse, err error)
	CreateLabel(ctx context.Context, request CreateLabelRequest) (response CreateLabelResponse, err error)
	UpdateLabel(ctx context.Context, request UpdateLabelRequest) (response UpdateLabelResponse, err error)
	DeleteLabel(ctx context.Context, request DeleteLabelRequest) (response DeleteLabelResponse, err error)
	GetLabelChats(ctx context.Context, request GetLabelChatsRequest) (response GetLabelChatsResponse, err error)
	SetChatLabel(ctx context.Context, request LabelChatRequest) (response LabelChatResponse, err error)
}
