package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/codehive/codehive/internal/models"
	"github.com/codehive/codehive/internal/store"
)

type Dispatcher struct {
	store      *store.WebhookStore
	httpClient *http.Client
	queue      chan deliveryJob
}

type deliveryJob struct {
	Webhook *models.Webhook
	Event   string
	Payload []byte
}

func NewDispatcher(ws *store.WebhookStore) *Dispatcher {
	d := &Dispatcher{
		store: ws,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		queue: make(chan deliveryJob, 1000),
	}
	for i := 0; i < 10; i++ {
		go d.worker()
	}
	return d
}

func (d *Dispatcher) Dispatch(ctx context.Context, repoID int64, event string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("webhook: failed to marshal payload: %v", err)
		return
	}

	hooks, err := d.store.ListByRepoAndEvent(ctx, repoID, event)
	if err != nil {
		log.Printf("webhook: failed to list hooks: %v", err)
		return
	}

	for _, hook := range hooks {
		select {
		case d.queue <- deliveryJob{Webhook: hook, Event: event, Payload: data}:
		default:
			log.Printf("webhook: queue full, dropping delivery for hook %d", hook.ID)
		}
	}
}

func (d *Dispatcher) worker() {
	for job := range d.queue {
		d.deliver(job, 0)
	}
}

func (d *Dispatcher) deliver(job deliveryJob, attempt int) {
	start := time.Now()

	req, err := http.NewRequest("POST", job.Webhook.URL, bytes.NewReader(job.Payload))
	if err != nil {
		log.Printf("webhook: invalid URL %s: %v", job.Webhook.URL, err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CodeHive-Event", job.Event)
	req.Header.Set("X-CodeHive-Delivery", fmt.Sprintf("%d-%d", job.Webhook.ID, start.UnixNano()))

	if job.Webhook.Secret != "" {
		mac := hmac.New(sha256.New, []byte(job.Webhook.Secret))
		mac.Write(job.Payload)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-CodeHive-Signature-256", "sha256="+sig)
	}

	resp, err := d.httpClient.Do(req)
	duration := int(time.Since(start).Milliseconds())

	delivery := &models.WebhookDelivery{
		WebhookID:  job.Webhook.ID,
		Event:      job.Event,
		Payload:    job.Payload,
		DurationMs: duration,
	}

	if err != nil {
		errMsg := err.Error()
		delivery.ResponseBody = &errMsg
		code := 0
		delivery.ResponseCode = &code
	} else {
		delivery.ResponseCode = &resp.StatusCode
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 10240))
		resp.Body.Close()
		bodyStr := string(body)
		delivery.ResponseBody = &bodyStr
	}

	d.store.RecordDelivery(context.Background(), delivery)

	// Retry on 5xx
	if delivery.ResponseCode != nil && *delivery.ResponseCode >= 500 && attempt < 3 {
		delays := []time.Duration{5 * time.Second, 30 * time.Second, 120 * time.Second}
		time.Sleep(delays[attempt])
		d.deliver(job, attempt+1)
	}
}
