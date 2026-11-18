package validations

import (
	"context"
	"errors"
	"regexp"
	"strings"

	domainLabel "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/label"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	validation "github.com/go-ozzo/ozzo-validation/v4"
)

// WhatsApp Business offers 20 label colors, addressed by index 0-19.
const (
	LabelColorMin int32 = 0
	LabelColorMax int32 = 19
)

// WhatsApp label ids are positive numeric strings.
var labelIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,17}$`)

var labelIDRules = []validation.Rule{
	validation.Required,
	validation.Match(labelIDPattern).Error("must be a positive numeric string"),
}

var labelColorRule = validation.By(func(value any) error {
	color, ok := value.(*int32)
	if !ok || color == nil {
		return nil
	}
	if *color < LabelColorMin || *color > LabelColorMax {
		return errors.New("must be between 0 and 19")
	}
	return nil
})

var notBlank = validation.By(func(value any) error {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return errors.New("cannot be blank")
		}
	case *string:
		if v != nil && strings.TrimSpace(*v) == "" {
			return errors.New("cannot be blank")
		}
	}
	return nil
})

func ValidateCreateLabel(ctx context.Context, request *domainLabel.CreateLabelRequest) error {
	err := validation.ValidateStructWithContext(ctx, request,
		validation.Field(&request.LabelID, validation.Match(labelIDPattern).Error("must be a positive numeric string")),
		validation.Field(&request.Name, validation.Required, notBlank),
		validation.Field(&request.Color, labelColorRule),
	)
	if err != nil {
		return pkgError.ValidationError(err.Error())
	}
	return nil
}

func ValidateUpdateLabel(ctx context.Context, request *domainLabel.UpdateLabelRequest) error {
	err := validation.ValidateStructWithContext(ctx, request,
		validation.Field(&request.LabelID, labelIDRules...),
		validation.Field(&request.Name, notBlank),
		validation.Field(&request.Color, labelColorRule),
	)
	if err != nil {
		return pkgError.ValidationError(err.Error())
	}
	if request.Name == nil && request.Color == nil {
		return pkgError.ValidationError("name or color is required")
	}
	return nil
}

func ValidateDeleteLabel(ctx context.Context, request *domainLabel.DeleteLabelRequest) error {
	err := validation.ValidateStructWithContext(ctx, request,
		validation.Field(&request.LabelID, labelIDRules...),
	)
	if err != nil {
		return pkgError.ValidationError(err.Error())
	}
	return nil
}

func ValidateGetLabelChats(ctx context.Context, request *domainLabel.GetLabelChatsRequest) error {
	err := validation.ValidateStructWithContext(ctx, request,
		validation.Field(&request.LabelID, labelIDRules...),
	)
	if err != nil {
		return pkgError.ValidationError(err.Error())
	}
	return nil
}

func ValidateLabelChat(ctx context.Context, request *domainLabel.LabelChatRequest) error {
	err := validation.ValidateStructWithContext(ctx, request,
		validation.Field(&request.LabelID, labelIDRules...),
		validation.Field(&request.ChatJID, validation.Required),
	)
	if err != nil {
		return pkgError.ValidationError(err.Error())
	}
	return nil
}
