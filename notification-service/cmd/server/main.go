package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/segmentio/kafka-go"
)

type BookingCreated struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	UserEmail string    `json:"user_email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

var (
	consulURL     = env("CONSUL_HTTP_ADDR", "http://consul:8500")
	kafkaBroker   = env("KAFKA_BROKER", "kafka:9092")
	bookingsTopic = env("BOOKINGS_TOPIC", "booking.created")
)

func main() {
	_ = registerInConsul()

	go consumeBookings(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "notification-service"})
	})

	server := &http.Server{
		Addr:              ":8004",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}
	log.Println("notification-service listening on :8004")
	log.Fatal(server.ListenAndServe())
}

func consumeBookings(ctx context.Context) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{kafkaBroker},
		Topic:          bookingsTopic,
		GroupID:        "notification-service",
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
	})
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("kafka read failed: %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		var event BookingCreated
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			log.Printf("invalid booking.created message: %v", err)
			continue
		}
		log.Printf("notification prepared for %s about booking %s for event %s", event.UserEmail, event.ID, event.EventID)
	}
}

func registerInConsul() error {
	payload := map[string]any{
		"ID":      "notification-service",
		"Name":    "notification-service",
		"Address": "notification-service",
		"Port":    8004,
		"Check": map[string]string{
			"HTTP":     "http://notification-service:8004/health",
			"Interval": "10s",
		},
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPut, consulURL+"/v1/agent/service/register", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
