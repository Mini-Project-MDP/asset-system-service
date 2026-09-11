package handler

import (
	"encoding/json"
	"log"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/delivery/http/middleware"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/domain"
	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

// WebhookHandler receives asynchronous state-change notifications from
// Approval-Engine-Service (see Langkah 6, asset-system-integration-checklist.md).
type WebhookHandler struct {
	service domain.RequestService
}

func NewWebhookHandler(requestService domain.RequestService) *WebhookHandler {
	return &WebhookHandler{service: requestService}
}

// ApprovalEngine handles POST /api/v1/webhooks/approval-engine. The route is
// unauthenticated by JWT (the caller is another service, not a logged-in
// user) — middleware.VerifyWebhookSignature gates it instead.
//
// @Summary Receive Approval Engine webhook
// @Description Internal callback invoked by Approval-Engine-Service when a request/step changes state. Requires a valid X-Webhook-Signature header.
// @Tags Webhooks
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Router /api/v1/webhooks/approval-engine [post]
func (h *WebhookHandler) ApprovalEngine(c fiber.Ctx) error {
	raw, _ := c.Locals(middleware.RawBodyContextKey).([]byte)

	var event domain.EngineWebhookEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return response.Error(c, fiber.StatusBadRequest, "Invalid webhook payload")
	}

	// Always acknowledge 200 once the signature is valid and the payload
	// parses: the engine's own delivery contract only retries on a non-2xx
	// response (see WebhookNotifier.attempt), and a transient failure to
	// refresh here is not something the engine retrying blindly would fix —
	// GET /requests/{id} remains the source of truth either way.
	if err := h.service.HandleEngineWebhook(c.Context(), event); err != nil {
		log.Printf("webhook: handle event %q for request %s failed: %v", event.Event, event.RequestID, err)
	}

	return response.Success(c, fiber.Map{"received": true})
}
