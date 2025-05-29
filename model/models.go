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

type Promocao struct {
	ID           string    `json:"id,omitempty"`           // Identificador único da promoção (opcional, pode ser gerado pelo MSMarketing)
	NomePromocao string    `json:"nomePromocao"`           // Nome chamativo da promoção
	CruiseID     string    `json:"cruiseId"`               // ID do cruzeiro ao qual a promoção se aplica
	Destino      string    `json:"destino,omitempty"`      // Destino para roteamento (MSMarketing ainda pode precisar disso)
	Descricao    string    `json:"descricao,omitempty"`    // Uma breve descrição (opcional)
	Timestamp    time.Time `json:"timestamp,omitempty"`    // Data e hora de criação/publicação da promoção
	PublicadoPor string    `json:"publicadoPor,omitempty"` // Identificador de quem publicou (ex: msmarketing)
}
