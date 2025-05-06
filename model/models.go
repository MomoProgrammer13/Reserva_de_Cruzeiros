package model

import (
	"time"
)

// ReservationRequest represents a request to reserve a cruise
type ReservationRequest struct {
	CruiseID      string  `json:"cruiseId"`
	ReservationID string  `json:"reservationId"`
	Customer      string  `json:"customer"`
	Passengers    int     `json:"passengers"`
	TotalPrice    float64 `json:"totalPrice"`
}

// ReservationResponse represents the response from a reservation request

// PaymentResponse represents the response from a payment request
type PaymentResponse struct {
	PaymentID     string    `json:"paymentId"`
	ReservationID string    `json:"reservationId"`
	CruiseID      string    `json:"cruiseId"`
	Customer      string    `json:"customer"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	IssueDate     time.Time `json:"issueDate"`
}

// BilheteResponse representa a resposta da geração de bilhete
type BilheteResponse struct {
	TicketID      string    `json:"ticketId"`
	ReservationID string    `json:"reservationId"`
	Customer      string    `json:"customer"`
	CruiseID      string    `json:"cruiseId"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	IssuedAt      time.Time `json:"issuedAt"`
}
