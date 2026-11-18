package validations

import (
	"context"
	"testing"

	domainLabel "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/label"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/stretchr/testify/assert"
)

func int32Ptr(v int32) *int32 { return &v }
func strPtr(v string) *string { return &v }

func assertValidation(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if !wantErr {
		assert.NoError(t, err)
		return
	}
	assert.Error(t, err)
	assert.IsType(t, pkgError.ValidationError(""), err)
}

func TestValidateCreateLabel(t *testing.T) {
	tests := []struct {
		name    string
		request domainLabel.CreateLabelRequest
		wantErr bool
	}{
		{"name only", domainLabel.CreateLabelRequest{Name: "VIP"}, false},
		{"explicit id and color", domainLabel.CreateLabelRequest{LabelID: "12", Name: "VIP", Color: int32Ptr(19)}, false},
		{"color zero is valid", domainLabel.CreateLabelRequest{Name: "VIP", Color: int32Ptr(0)}, false},
		{"missing name", domainLabel.CreateLabelRequest{}, true},
		{"blank name", domainLabel.CreateLabelRequest{Name: "   "}, true},
		{"non numeric id", domainLabel.CreateLabelRequest{LabelID: "abc", Name: "VIP"}, true},
		{"zero id", domainLabel.CreateLabelRequest{LabelID: "0", Name: "VIP"}, true},
		{"color above range", domainLabel.CreateLabelRequest{Name: "VIP", Color: int32Ptr(20)}, true},
		{"negative color", domainLabel.CreateLabelRequest{Name: "VIP", Color: int32Ptr(-1)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertValidation(t, ValidateCreateLabel(context.Background(), &tt.request), tt.wantErr)
		})
	}
}

func TestValidateUpdateLabel(t *testing.T) {
	tests := []struct {
		name    string
		request domainLabel.UpdateLabelRequest
		wantErr bool
	}{
		{"name only", domainLabel.UpdateLabelRequest{LabelID: "3", Name: strPtr("New")}, false},
		{"color only", domainLabel.UpdateLabelRequest{LabelID: "3", Color: int32Ptr(0)}, false},
		{"neither field", domainLabel.UpdateLabelRequest{LabelID: "3"}, true},
		{"blank name", domainLabel.UpdateLabelRequest{LabelID: "3", Name: strPtr(" ")}, true},
		{"missing id", domainLabel.UpdateLabelRequest{Name: strPtr("New")}, true},
		{"bad color", domainLabel.UpdateLabelRequest{LabelID: "3", Color: int32Ptr(42)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertValidation(t, ValidateUpdateLabel(context.Background(), &tt.request), tt.wantErr)
		})
	}
}

func TestValidateLabelIDAndChatRequests(t *testing.T) {
	assertValidation(t, ValidateDeleteLabel(context.Background(), &domainLabel.DeleteLabelRequest{LabelID: "5"}), false)
	assertValidation(t, ValidateDeleteLabel(context.Background(), &domainLabel.DeleteLabelRequest{LabelID: "5a"}), true)
	assertValidation(t, ValidateGetLabelChats(context.Background(), &domainLabel.GetLabelChatsRequest{LabelID: "5"}), false)
	assertValidation(t, ValidateGetLabelChats(context.Background(), &domainLabel.GetLabelChatsRequest{}), true)
	assertValidation(t, ValidateLabelChat(context.Background(), &domainLabel.LabelChatRequest{LabelID: "5", ChatJID: "628123456789@s.whatsapp.net"}), false)
	assertValidation(t, ValidateLabelChat(context.Background(), &domainLabel.LabelChatRequest{LabelID: "5"}), true)
}
