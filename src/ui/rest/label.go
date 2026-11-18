package rest

import (
	"strings"

	domainLabel "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/label"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/gofiber/fiber/v3"
)

// Label exposes WhatsApp Business label management. Labels only exist on
// WhatsApp Business accounts.
type Label struct {
	Service domainLabel.ILabelUsecase
}

func InitRestLabel(app fiber.Router, service domainLabel.ILabelUsecase) Label {
	rest := Label{Service: service}

	app.Get("/labels", rest.ListLabels)
	app.Post("/label", rest.CreateLabel)
	app.Patch("/label/:label_id", rest.UpdateLabel)
	app.Delete("/label/:label_id", rest.DeleteLabel)
	app.Get("/label/:label_id/chats", rest.GetLabelChats)
	app.Post("/label/:label_id/chat/:chat_jid", rest.AddChatLabel)
	app.Delete("/label/:label_id/chat/:chat_jid", rest.RemoveChatLabel)

	return rest
}

// labelIDParam detaches the label_id path parameter from fiber's reusable buffer.
func labelIDParam(c fiber.Ctx) string {
	return strings.Clone(c.Params("label_id"))
}

func badLabelRequest(c fiber.Ctx, message string) error {
	return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
		Status:  fiber.StatusBadRequest,
		Code:    "BAD_REQUEST",
		Message: message,
		Results: nil,
	})
}

func (controller *Label) ListLabels(c fiber.Ctx) error {
	response, err := controller.Service.ListLabels(whatsapp.ContextWithDevice(c.Context(), getDeviceFromCtx(c)))
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Success get label list",
		Results: response,
	})
}

func (controller *Label) CreateLabel(c fiber.Ctx) error {
	var request domainLabel.CreateLabelRequest
	if err := c.Bind().Body(&request); err != nil {
		return badLabelRequest(c, "Invalid request body")
	}

	response, err := controller.Service.CreateLabel(whatsapp.ContextWithDevice(c.Context(), getDeviceFromCtx(c)), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Message,
		Results: response,
	})
}

func (controller *Label) UpdateLabel(c fiber.Ctx) error {
	var request domainLabel.UpdateLabelRequest
	if err := c.Bind().Body(&request); err != nil {
		return badLabelRequest(c, "Invalid request body")
	}
	// Route id is authoritative; a body label_id must never override it.
	request.LabelID = labelIDParam(c)

	response, err := controller.Service.UpdateLabel(whatsapp.ContextWithDevice(c.Context(), getDeviceFromCtx(c)), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Message,
		Results: response,
	})
}

func (controller *Label) DeleteLabel(c fiber.Ctx) error {
	request := domainLabel.DeleteLabelRequest{LabelID: labelIDParam(c)}

	response, err := controller.Service.DeleteLabel(whatsapp.ContextWithDevice(c.Context(), getDeviceFromCtx(c)), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Message,
		Results: response,
	})
}

func (controller *Label) GetLabelChats(c fiber.Ctx) error {
	request := domainLabel.GetLabelChatsRequest{LabelID: labelIDParam(c)}

	response, err := controller.Service.GetLabelChats(whatsapp.ContextWithDevice(c.Context(), getDeviceFromCtx(c)), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Success get label chats",
		Results: response,
	})
}

func (controller *Label) AddChatLabel(c fiber.Ctx) error {
	return controller.setChatLabel(c, true)
}

func (controller *Label) RemoveChatLabel(c fiber.Ctx) error {
	return controller.setChatLabel(c, false)
}

func (controller *Label) setChatLabel(c fiber.Ctx, labeled bool) error {
	chatJID, err := chatJIDParam(c)
	if err != nil {
		return badLabelRequest(c, "invalid chat_jid path parameter: "+err.Error())
	}
	request := domainLabel.LabelChatRequest{
		LabelID: labelIDParam(c),
		ChatJID: chatJID,
		Labeled: labeled,
	}

	response, err := controller.Service.SetChatLabel(whatsapp.ContextWithDevice(c.Context(), getDeviceFromCtx(c)), request)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: response.Message,
		Results: response,
	})
}
