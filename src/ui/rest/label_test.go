package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	domainLabel "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/label"
	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/middleware"
	"github.com/gofiber/fiber/v3"
	"github.com/stretchr/testify/require"
)

// labelUsecaseSpy records the last call so the REST handlers can be tested alone.
type labelUsecaseSpy struct {
	calls     []string
	create    domainLabel.CreateLabelRequest
	update    domainLabel.UpdateLabelRequest
	deleted   domainLabel.DeleteLabelRequest
	chats     domainLabel.GetLabelChatsRequest
	chatLabel domainLabel.LabelChatRequest
	err       error
}

func (s *labelUsecaseSpy) ListLabels(context.Context) (domainLabel.ListLabelsResponse, error) {
	s.calls = append(s.calls, "list")
	return domainLabel.ListLabelsResponse{Data: []domainLabel.LabelInfo{{ID: "1", Name: "VIP", Color: 2}}}, s.err
}

func (s *labelUsecaseSpy) CreateLabel(_ context.Context, r domainLabel.CreateLabelRequest) (domainLabel.CreateLabelResponse, error) {
	s.calls = append(s.calls, "create")
	s.create = r
	return domainLabel.CreateLabelResponse{Status: "success", Label: domainLabel.LabelInfo{ID: "4", Name: r.Name}}, s.err
}

func (s *labelUsecaseSpy) UpdateLabel(_ context.Context, r domainLabel.UpdateLabelRequest) (domainLabel.UpdateLabelResponse, error) {
	s.calls = append(s.calls, "update")
	s.update = r
	return domainLabel.UpdateLabelResponse{}, s.err
}

func (s *labelUsecaseSpy) DeleteLabel(_ context.Context, r domainLabel.DeleteLabelRequest) (domainLabel.DeleteLabelResponse, error) {
	s.calls = append(s.calls, "delete")
	s.deleted = r
	return domainLabel.DeleteLabelResponse{}, s.err
}

func (s *labelUsecaseSpy) GetLabelChats(_ context.Context, r domainLabel.GetLabelChatsRequest) (domainLabel.GetLabelChatsResponse, error) {
	s.calls = append(s.calls, "chats")
	s.chats = r
	return domainLabel.GetLabelChatsResponse{}, s.err
}

func (s *labelUsecaseSpy) SetChatLabel(_ context.Context, r domainLabel.LabelChatRequest) (domainLabel.LabelChatResponse, error) {
	s.calls = append(s.calls, "chat")
	s.chatLabel = r
	return domainLabel.LabelChatResponse{}, s.err
}

func newLabelTestApp(spy *labelUsecaseSpy) *fiber.App {
	app := fiber.New()
	app.Use(middleware.Recovery())
	InitRestLabel(app, spy)
	return app
}

func doLabelRequest(t *testing.T, app *fiber.App, method, path, body string) (int, utils.ResponseData) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	var envelope utils.ResponseData
	require.NoError(t, json.Unmarshal(raw, &envelope), "body: %s", raw)
	return resp.StatusCode, envelope
}

func TestLabelRoutesDispatchToUsecase(t *testing.T) {
	spy := &labelUsecaseSpy{}
	app := newLabelTestApp(spy)

	status, envelope := doLabelRequest(t, app, "GET", "/labels", "")
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, "SUCCESS", envelope.Code)

	status, _ = doLabelRequest(t, app, "POST", "/label", `{"label_id":"12","name":"VIP","color":0}`)
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, "12", spy.create.LabelID)
	require.Equal(t, "VIP", spy.create.Name)
	require.NotNil(t, spy.create.Color, "an explicit color 0 must reach the usecase")
	require.Equal(t, int32(0), *spy.create.Color)

	status, _ = doLabelRequest(t, app, "PATCH", "/label/7", `{"label_id":"99","name":"Renamed"}`)
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, "7", spy.update.LabelID, "route label_id must override the body")
	require.NotNil(t, spy.update.Name)
	require.Nil(t, spy.update.Color, "an omitted color must stay nil")

	status, _ = doLabelRequest(t, app, "DELETE", "/label/7", "")
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, "7", spy.deleted.LabelID)

	status, _ = doLabelRequest(t, app, "GET", "/label/7/chats", "")
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, "7", spy.chats.LabelID)

	status, _ = doLabelRequest(t, app, "POST", "/label/7/chat/120363151317289139%40g.us", "")
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, domainLabel.LabelChatRequest{LabelID: "7", ChatJID: "120363151317289139@g.us", Labeled: true}, spy.chatLabel)

	status, _ = doLabelRequest(t, app, "DELETE", "/label/7/chat/6289685028129@s.whatsapp.net", "")
	require.Equal(t, fiber.StatusOK, status)
	require.Equal(t, domainLabel.LabelChatRequest{LabelID: "7", ChatJID: "6289685028129@s.whatsapp.net", Labeled: false}, spy.chatLabel)

	require.Equal(t, []string{"list", "create", "update", "delete", "chats", "chat", "chat"}, spy.calls)
}

func TestLabelNotFoundMapsTo404(t *testing.T) {
	spy := &labelUsecaseSpy{err: pkgError.ErrLabelNotFound}
	app := newLabelTestApp(spy)

	status, envelope := doLabelRequest(t, app, "PATCH", "/label/7", `{"name":"x"}`)
	require.Equal(t, fiber.StatusNotFound, status)
	require.Equal(t, "NOT_FOUND", envelope.Code)
}

func TestLabelRejectsMalformedBody(t *testing.T) {
	spy := &labelUsecaseSpy{}
	app := newLabelTestApp(spy)

	status, envelope := doLabelRequest(t, app, "POST", "/label", `{"name":`)
	require.Equal(t, fiber.StatusBadRequest, status)
	require.Equal(t, "BAD_REQUEST", envelope.Code)
	require.Empty(t, spy.calls)
}
