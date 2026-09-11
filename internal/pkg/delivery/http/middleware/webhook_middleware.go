package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"

	"github.com/Mini-Project-MDP/asset-system-service/internal/pkg/response"
	"github.com/gofiber/fiber/v3"
)

// RawBodyContextKey stores the raw request body captured before Fiber's
// body-parsing runs, so the webhook handler downstream can unmarshal the
// exact bytes whose signature was just verified rather than re-reading the
// body (and risking a mismatch from any parsing/re-encoding).
const RawBodyContextKey = "webhookRawBody"

// VerifyWebhookSignature returns a Fiber middleware that authenticates a
// Approval-Engine-Service webhook delivery: the raw body must hash to the
// hex HMAC-SHA256 in the X-Webhook-Signature header ("sha256=<hex>"), keyed
// with secret — the same api_key issued to this application at registration
// (see Approval-Engine-Service/internal/service/webhook.go's sign()). The
// callback URL itself carries no other authentication, so a request that
// fails this check is rejected before it ever reaches the handler/service.
func VerifyWebhookSignature(secret string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if secret == "" {
			// Not configured — refuse rather than silently accept
			// unauthenticated webhooks.
			return response.Error(c, fiber.StatusServiceUnavailable, "Webhook receiver is not configured")
		}

		signature := c.Get("X-Webhook-Signature")
		if signature == "" {
			return response.Error(c, fiber.StatusUnauthorized, "Missing X-Webhook-Signature header")
		}

		body := c.Req().Body()
		if !validSignature(body, secret, signature) {
			return response.Error(c, fiber.StatusUnauthorized, "Invalid webhook signature")
		}

		c.Locals(RawBodyContextKey, body)
		return c.Next()
	}
}

func validSignature(body []byte, secret, header string) bool {
	const prefix = "sha256="
	hexDigest, ok := strings.CutPrefix(header, prefix)
	if !ok {
		return false
	}
	given, err := hex.DecodeString(hexDigest)
	if err != nil {
		return false
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	return subtle.ConstantTimeCompare(given, expected) == 1
}
