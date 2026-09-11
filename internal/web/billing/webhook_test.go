// internal/web/billing/webhook_test.go
package billing

import (
	"bytes"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	webdb "github.com/jbrahy/AntiVirus/internal/web/db"
	"github.com/stripe/stripe-go/v82"
	"github.com/stripe/stripe-go/v82/webhook"
)

const testWebhookSecret = "whsec_test_secret"

// mysqlDateTime is MySQL's DATETIME literal layout, used to compare a
// scanned time.Time against a literal written into the column.
const mysqlDateTime = "2006-01-02 15:04:05"

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DB_DSN")
	if dsn == "" {
		dsn = "root@tcp(127.0.0.1:3306)/avtool_web_test"
	}
	d, err := webdb.Open(dsn)
	if err != nil {
		t.Skipf("no reachable test MariaDB, skipping: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

// uniqueSuffix returns a random hex string unique to this test invocation,
// since the shared test database is not wiped between runs. It is used to
// derive per-test emails, Stripe customer IDs, and Stripe subscription IDs
// so that: (a) users.email (UNIQUE) and subscriptions.stripe_subscription_id
// (UNIQUE) don't collide across runs, and (b) the users.stripe_customer_id ->
// user lookup in upsertSubscription (which has no UNIQUE constraint to lean
// on) resolves to the row this test just created rather than a same-named
// row left over from an earlier run.
func uniqueSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("generating unique suffix: %v", err)
	}
	return hex.EncodeToString(b)
}

// subscriptionEventPayload builds a minimal JSON payload for a
// customer.subscription.{created,updated,deleted} event, matching the shape
// Stripe actually sends: "customer" as a plain (non-expanded) customer ID
// string, current_period_end nested under items.data[0] rather than
// directly on the subscription object (see webhook.go for why), and an
// "api_version" matching the compiled-in SDK version. Real Stripe webhook
// deliveries always carry api_version, and webhook.ConstructEvent rejects
// events whose api_version release train doesn't match stripe.APIVersion
// (confirmed by calling ConstructEvent directly against a payload missing
// this field: "Received event with API version , but stripe-go 82.5.1
// expects API version 2025-08-27.basil").
func subscriptionEventPayload(eventType, subscriptionID, customerID, status string, currentPeriodEnd int64) []byte {
	payload := map[string]any{
		"id":          "evt_test",
		"object":      "event",
		"type":        eventType,
		"api_version": stripe.APIVersion,
		"data": map[string]any{
			"object": map[string]any{
				"id":       subscriptionID,
				"object":   "subscription",
				"customer": customerID,
				"status":   status,
				"items": map[string]any{
					"object": "list",
					"data": []map[string]any{
						{
							"id":                   "si_test",
							"object":               "subscription_item",
							"current_period_end":   currentPeriodEnd,
							"current_period_start": currentPeriodEnd - 2592000,
						},
					},
				},
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return b
}

func postSignedWebhook(t *testing.T, handler http.HandlerFunc, payload []byte) *httptest.ResponseRecorder {
	t.Helper()
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  testWebhookSecret,
	})

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(signed.Payload))
	req.Header.Set("Stripe-Signature", signed.Header)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestHandleWebhookRejectsBadSignature(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewBufferString(`{"type":"customer.subscription.created"}`))
	req.Header.Set("Stripe-Signature", "t=1,v1=deadbeef")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a bad signature", rec.Code)
	}
}

func insertUserWithStripeCustomerID(t *testing.T, d *sql.DB, email, customerID string) uint64 {
	t.Helper()
	res, err := d.Exec(`INSERT INTO users (email, password_hash, stripe_customer_id) VALUES (?, ?, ?)`,
		email, "irrelevant-hash", customerID)
	if err != nil {
		t.Fatalf("inserting user: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId: %v", err)
	}
	return uint64(id)
}

func TestHandleWebhookUpsertsActiveSubscriptionAndGeneratesLicense(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	suffix := uniqueSuffix(t)
	customerID := "cus_test_active_" + suffix
	subscriptionID := "sub_test_active_" + suffix
	userID := insertUserWithStripeCustomerID(t, d, fmt.Sprintf("sub-active-%s@example.com", suffix), customerID)

	payload := subscriptionEventPayload("customer.subscription.created", subscriptionID, customerID, "active", 1893456000)
	rec := postSignedWebhook(t, handler, payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var status string
	var stripeCustomerID string
	err := d.QueryRow(`SELECT status, stripe_customer_id FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).
		Scan(&status, &stripeCustomerID)
	if err != nil {
		t.Fatalf("querying subscriptions: %v", err)
	}
	if status != "active" {
		t.Fatalf("status = %q, want %q", status, "active")
	}
	if stripeCustomerID != customerID {
		t.Fatalf("stripe_customer_id = %q, want %q", stripeCustomerID, customerID)
	}

	var licenseCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM licenses WHERE user_id = ?`, userID).Scan(&licenseCount); err != nil {
		t.Fatalf("counting licenses: %v", err)
	}
	if licenseCount != 1 {
		t.Fatalf("license count = %d, want 1", licenseCount)
	}
}

func TestHandleWebhookDoesNotRegenerateLicenseOnRepeatedActiveEvents(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	suffix := uniqueSuffix(t)
	customerID := "cus_test_repeat_" + suffix
	subscriptionID := "sub_test_repeat_" + suffix
	userID := insertUserWithStripeCustomerID(t, d, fmt.Sprintf("sub-repeat-%s@example.com", suffix), customerID)

	created := subscriptionEventPayload("customer.subscription.created", subscriptionID, customerID, "active", 1893456000)
	if rec := postSignedWebhook(t, handler, created); rec.Code != http.StatusOK {
		t.Fatalf("created event: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	updated := subscriptionEventPayload("customer.subscription.updated", subscriptionID, customerID, "active", 1896134400)
	if rec := postSignedWebhook(t, handler, updated); rec.Code != http.StatusOK {
		t.Fatalf("updated event: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var licenseCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM licenses WHERE user_id = ?`, userID).Scan(&licenseCount); err != nil {
		t.Fatalf("counting licenses: %v", err)
	}
	if licenseCount != 1 {
		t.Fatalf("license count after created+updated = %d, want exactly 1 (regression: license regenerated on retry/update)", licenseCount)
	}

	var subCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).Scan(&subCount); err != nil {
		t.Fatalf("counting subscriptions: %v", err)
	}
	if subCount != 1 {
		t.Fatalf("subscription row count = %d, want exactly 1 (upsert should not duplicate)", subCount)
	}
}

func TestHandleWebhookMarksCanceledOnSubscriptionDeleted(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	suffix := uniqueSuffix(t)
	customerID := "cus_test_cancel_" + suffix
	subscriptionID := "sub_test_cancel_" + suffix
	userID := insertUserWithStripeCustomerID(t, d, fmt.Sprintf("sub-cancel-%s@example.com", suffix), customerID)

	created := subscriptionEventPayload("customer.subscription.created", subscriptionID, customerID, "active", 1893456000)
	if rec := postSignedWebhook(t, handler, created); rec.Code != http.StatusOK {
		t.Fatalf("created event: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	deleted := subscriptionEventPayload("customer.subscription.deleted", subscriptionID, customerID, "canceled", 1893456000)
	rec := postSignedWebhook(t, handler, deleted)
	if rec.Code != http.StatusOK {
		t.Fatalf("deleted event: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var status string
	if err := d.QueryRow(`SELECT status FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).Scan(&status); err != nil {
		t.Fatalf("querying subscriptions: %v", err)
	}
	if status != "canceled" {
		t.Fatalf("status = %q, want %q", status, "canceled")
	}

	// Regression coverage: subscription.deleted must also revoke the
	// license generated when the subscription went active. Before this
	// change, cancellation only updated the subscriptions row, leaving a
	// canceled customer's license permanently valid.
	var revokedCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM licenses WHERE user_id = ? AND revoked_at IS NOT NULL`, userID).Scan(&revokedCount); err != nil {
		t.Fatalf("counting revoked licenses: %v", err)
	}
	if revokedCount != 1 {
		t.Fatalf("revoked license count = %d, want 1 (license should be revoked on cancellation)", revokedCount)
	}
}

func TestHandleWebhookUnknownSubscriptionDeletedSucceeds(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	// Same reasoning as TestHandleWebhookUnknownCustomerSucceeds: this
	// Stripe account is shared with other products, so a deleted event for
	// a subscription this table never had a row for is expected traffic,
	// not an error, and must not 500 (Stripe retries a 500 for up to 3
	// days and can eventually disable the endpoint).
	payload := subscriptionEventPayload("customer.subscription.deleted", "sub_never_seen", "cus_never_seen", "canceled", 1893456000)
	rec := postSignedWebhook(t, handler, payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a deleted event about an unknown subscription; body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandleWebhookPaymentFailedSucceeds(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	suffix := uniqueSuffix(t)
	payload := map[string]any{
		"id":          "evt_test_payment_failed",
		"object":      "event",
		"type":        "invoice.payment_failed",
		"api_version": stripe.APIVersion,
		"data": map[string]any{
			"object": map[string]any{
				"id":       "in_test_" + suffix,
				"object":   "invoice",
				"customer": "cus_test_" + suffix,
			},
		},
	}
	b, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshaling payload: %v", err)
	}

	rec := postSignedWebhook(t, handler, b)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
}

// TestHandleWebhookUnknownCustomerSucceeds is a regression test for a
// whole-branch review finding: this Stripe account is shared with other,
// unrelated products (AudioPanels, wearz.com), so events about customers we
// have no local mapping for are expected and must not 500 — a persistent
// 500 risks Stripe retrying for up to 3 days and then disabling the
// endpoint, which would silently break license issuance for real avtool
// customers too. The correct response for "not one of our customers" is 200
// (nothing to retry), not 500.
func TestHandleWebhookUnknownCustomerSucceeds(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	payload := subscriptionEventPayload("customer.subscription.created", "sub_test_unknown", "cus_does_not_exist", "active", 1893456000)
	rec := postSignedWebhook(t, handler, payload)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for an event about an unmapped stripe customer (should not be retried by Stripe); body = %s", rec.Code, rec.Body.String())
	}
}

// signedDelivery captures one signed Stripe delivery so the exact same body
// and Stripe-Signature header can be posted more than once. postSignedWebhook
// re-signs on every call, which produces a fresh signature each time; a real
// duplicate redelivery from Stripe is the identical bytes, identical event ID
// and identical signature arriving twice, so idempotency tests need this
// instead.
type signedDelivery struct {
	body   []byte
	header string
}

func signOnce(payload []byte) signedDelivery {
	signed := webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{
		Payload: payload,
		Secret:  testWebhookSecret,
	})
	return signedDelivery{body: signed.Payload, header: signed.Header}
}

func (d signedDelivery) post(t *testing.T, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(d.body))
	req.Header.Set("Stripe-Signature", d.header)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

// TestHandleWebhookExactDuplicateCreatedEventIsIdempotent posts one signed
// customer.subscription.created delivery twice, byte for byte. Stripe
// redelivers on timeout or a non-2xx response and can redeliver the same
// event ID even after a 200, so the second delivery must leave the database
// exactly as the first did: one subscription row, one license, no error.
func TestHandleWebhookExactDuplicateCreatedEventIsIdempotent(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	suffix := uniqueSuffix(t)
	customerID := "cus_test_dup_" + suffix
	subscriptionID := "sub_test_dup_" + suffix
	userID := insertUserWithStripeCustomerID(t, d, fmt.Sprintf("sub-dup-%s@example.com", suffix), customerID)

	delivery := signOnce(subscriptionEventPayload("customer.subscription.created", subscriptionID, customerID, "active", 1893456000))

	if rec := delivery.post(t, handler); rec.Code != http.StatusOK {
		t.Fatalf("first delivery: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var firstStatus, firstPeriodEnd string
	if err := d.QueryRow(`SELECT status, current_period_end FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).
		Scan(&firstStatus, &firstPeriodEnd); err != nil {
		t.Fatalf("querying subscription after first delivery: %v", err)
	}

	if rec := delivery.post(t, handler); rec.Code != http.StatusOK {
		t.Fatalf("duplicate delivery: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var subCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).Scan(&subCount); err != nil {
		t.Fatalf("counting subscriptions: %v", err)
	}
	if subCount != 1 {
		t.Fatalf("subscription row count = %d, want exactly 1 after a duplicate delivery", subCount)
	}

	var secondStatus, secondPeriodEnd string
	if err := d.QueryRow(`SELECT status, current_period_end FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).
		Scan(&secondStatus, &secondPeriodEnd); err != nil {
		t.Fatalf("querying subscription after duplicate delivery: %v", err)
	}
	if secondStatus != firstStatus || secondPeriodEnd != firstPeriodEnd {
		t.Fatalf("subscription changed on duplicate delivery: (%q, %q) -> (%q, %q)",
			firstStatus, firstPeriodEnd, secondStatus, secondPeriodEnd)
	}

	var licenseCount int
	if err := d.QueryRow(`SELECT COUNT(*) FROM licenses WHERE user_id = ?`, userID).Scan(&licenseCount); err != nil {
		t.Fatalf("counting licenses: %v", err)
	}
	if licenseCount != 1 {
		t.Fatalf("license count = %d, want exactly 1 (duplicate delivery generated a second license)", licenseCount)
	}
}

// TestHandleWebhookExactDuplicateDeletedEventDoesNotReRevoke posts one signed
// customer.subscription.deleted delivery twice. The revocation timestamp is
// backdated between the two deliveries so a re-revoke is observable: NOW()
// has one-second resolution and both deliveries land inside the same second,
// so without backdating an UPDATE that re-revokes would be indistinguishable
// from one that correctly does nothing.
func TestHandleWebhookExactDuplicateDeletedEventDoesNotReRevoke(t *testing.T) {
	d := testDB(t)
	handler := HandleWebhook(d, testWebhookSecret)

	suffix := uniqueSuffix(t)
	customerID := "cus_test_dupdel_" + suffix
	subscriptionID := "sub_test_dupdel_" + suffix
	userID := insertUserWithStripeCustomerID(t, d, fmt.Sprintf("sub-dupdel-%s@example.com", suffix), customerID)

	created := subscriptionEventPayload("customer.subscription.created", subscriptionID, customerID, "active", 1893456000)
	if rec := postSignedWebhook(t, handler, created); rec.Code != http.StatusOK {
		t.Fatalf("created event: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	delivery := signOnce(subscriptionEventPayload("customer.subscription.deleted", subscriptionID, customerID, "canceled", 1893456000))
	if rec := delivery.post(t, handler); rec.Code != http.StatusOK {
		t.Fatalf("first deleted delivery: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	const backdated = "2020-01-02 03:04:05"
	if _, err := d.Exec(`UPDATE licenses SET revoked_at = ? WHERE user_id = ?`, backdated, userID); err != nil {
		t.Fatalf("backdating revoked_at: %v", err)
	}

	if rec := delivery.post(t, handler); rec.Code != http.StatusOK {
		t.Fatalf("duplicate deleted delivery: status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	// internal/web/db.Open sets parseTime, so a DATETIME scans into
	// time.Time; compare the stored wall-clock value rather than the
	// driver's rendering of it.
	var revokedAt time.Time
	if err := d.QueryRow(`SELECT revoked_at FROM licenses WHERE user_id = ?`, userID).Scan(&revokedAt); err != nil {
		t.Fatalf("querying revoked_at: %v", err)
	}
	if got := revokedAt.Format(mysqlDateTime); got != backdated {
		t.Fatalf("revoked_at = %q, want %q unchanged (duplicate delivery re-revoked an already-revoked license)", got, backdated)
	}

	var status string
	if err := d.QueryRow(`SELECT status FROM subscriptions WHERE stripe_subscription_id = ?`, subscriptionID).Scan(&status); err != nil {
		t.Fatalf("querying subscription status: %v", err)
	}
	if status != "canceled" {
		t.Fatalf("status = %q, want %q after a duplicate deleted delivery", status, "canceled")
	}
}
