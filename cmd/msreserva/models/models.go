package models

import (
	"time"
)

// ReservationRequest represents a request to reserve a cruise
type ReservationRequest struct {
	CruiseID      string    `json:"cruiseId"`
	CustomerID    string    `json:"customerId"`
	Passengers    int       `json:"passengers"`
	CabinType     string    `json:"cabinType"`
	DepartureDate time.Time `json:"departureDate"`
	TotalPrice    float64   `json:"totalPrice"`
}

// ReservationResponse represents the response from a reservation request
type ReservationResponse struct {
	ReservationID string    `json:"reservationId"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	CreatedAt     time.Time `json:"createdAt"`
}

// PaymentRequest represents a request to process payment for a reservation
type PaymentRequest struct {
	ReservationID  string  `json:"reservationId"`
	Amount         float64 `json:"amount"`
	CardNumber     string  `json:"cardNumber"`
	CardHolderName string  `json:"cardHolderName"`
	ExpiryDate     string  `json:"expiryDate"`
	CVV            string  `json:"cvv"`
}

// PaymentResponse represents the response from a payment request
type PaymentResponse struct {
	PaymentID     string    `json:"paymentId"`
	ReservationID string    `json:"reservationId"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	ProcessedAt   time.Time `json:"processedAt"`
}
