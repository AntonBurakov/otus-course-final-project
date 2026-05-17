package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
)

type BookingRequest struct {
	EventID string `json:"event_id"`
}

type Booking struct {
	ID        string    `json:"id"`
	EventID   string    `json:"event_id"`
	UserEmail string    `json:"user_email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	ID       string `json:"id"`
	Capacity int    `json:"capacity"`
	Booked   int    `json:"booked"`
}

type User struct {
	Email string `json:"email"`
}

var (
	authURL       = env("AUTH_SERVICE_URL", "http://auth-service:8001")
	eventsURL     = env("EVENTS_SERVICE_URL", "http://events-service:8002")
	consulURL     = env("CONSUL_HTTP_ADDR", "http://consul:8500")
	kafkaBroker   = env("KAFKA_BROKER", "kafka:9092")
	bookingsTopic = env("BOOKINGS_TOPIC", "booking.created")
	store         = map[string]Booking{}
	storeMu       sync.RWMutex
)

func main() {
	_ = registerInConsul()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)
	mux.HandleFunc("POST /bookings", createBookingHandler)
	mux.HandleFunc("GET /bookings/{id}", getBookingHandler)

	server := &http.Server{
		Addr:              ":8003",
		Handler:           logging(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("booking-service listening on :8003")
	log.Fatal(server.ListenAndServe())
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "booking-service"})
}

func createBookingHandler(w http.ResponseWriter, r *http.Request) {
	user, err := verifyUser(r.Header.Get("Authorization"))
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"detail": err.Error()})
		return
	}

	var payload BookingRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.EventID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"detail": "event_id is required"})
		return
	}

	event, err := fetchEvent(payload.EventID)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"detail": err.Error()})
		return
	}
	if event.Booked >= event.Capacity {
		writeJSON(w, http.StatusConflict, map[string]string{"detail": "event is sold out"})
		return
	}

	booking := Booking{
		ID:        fmt.Sprintf("bkg-%d", time.Now().UnixNano()),
		EventID:   payload.EventID,
		UserEmail: user.Email,
		Status:    "confirmed",
		CreatedAt: time.Now().UTC(),
	}

	storeMu.Lock()
	store[booking.ID] = booking
	storeMu.Unlock()

	if err := publishBookingCreated(r.Context(), booking); err != nil {
		log.Printf("publish booking.created failed: %v", err)
	}

	writeJSON(w, http.StatusCreated, booking)
}

func getBookingHandler(w http.ResponseWriter, r *http.Request) {
	if _, err := verifyUser(r.Header.Get("Authorization")); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"detail": err.Error()})
		return
	}

	id := r.PathValue("id")
	storeMu.RLock()
	booking, ok := store[id]
	storeMu.RUnlock()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"detail": "booking not found"})
		return
	}
	writeJSON(w, http.StatusOK, booking)
}

func verifyUser(authHeader string) (User, error) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return User{}, errors.New("missing token")
	}

	req, err := http.NewRequest(http.MethodGet, authURL+"/verify", nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Authorization", authHeader)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return User{}, errors.New("invalid token")
	}

	var user User
	return user, json.NewDecoder(resp.Body).Decode(&user)
}

func fetchEvent(eventID string) (Event, error) {
	resp, err := http.Get(eventsURL + "/events/" + eventID)
	if err != nil {
		return Event{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Event{}, fmt.Errorf("event service returned %d", resp.StatusCode)
	}

	var event Event
	return event, json.NewDecoder(resp.Body).Decode(&event)
}

func publishBookingCreated(ctx context.Context, booking Booking) error {
	body, err := json.Marshal(booking)
	if err != nil {
		return err
	}
	writer := kafka.Writer{
		Addr:         kafka.TCP(kafkaBroker),
		Topic:        bookingsTopic,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	defer writer.Close()

	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(booking.ID),
		Value: body,
		Time:  time.Now(),
	})
}

func registerInConsul() error {
	payload := map[string]any{
		"ID":      "booking-service",
		"Name":    "booking-service",
		"Address": "booking-service",
		"Port":    8003,
		"Check": map[string]string{
			"HTTP":     "http://booking-service:8003/health",
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

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
