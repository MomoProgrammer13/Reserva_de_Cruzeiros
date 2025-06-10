package model

import (
	"time"
)

// ItineraryQuery representa os parâmetros de busca para um itinerário.
type ItineraryQuery struct {
	Destino       string `json:"destino"`
	DataEmbarque  string `json:"dataEmbarque"`
	PortoEmbarque string `json:"portoEmbarque"`
}

// ReservationRequest representa os dados base de uma reserva.
type ReservationRequest struct {
	CruiseID      int     `json:"cruiseId"`
	Customer      string  `json:"customer"`
	Passengers    int     `json:"passengers"`
	NumeroCabines int     `json:"numeroCabines"`
	DataEmbarque  string  `json:"dataEmbarque"`
	ValorTotal    float64 `json:"valorTotal"`
}

// ReservationRequestWithClient é o payload completo que vem do frontend, incluindo o ID da sessão SSE.
type ReservationRequestWithClient struct {
	ReservationRequest
	ClientID string `json:"clientId"`
}

// ReservationCreatedEvent é o evento publicado quando uma reserva é criada.
type ReservationCreatedEvent struct {
	ReservationID string `json:"reservationId"`
	ReservationRequest
}

// CancelRequest representa a requisição para cancelar uma reserva.
type CancelRequest struct {
	ReservationID string `json:"reservationId"`
}

// ReservationCancelledEvent é o evento publicado quando uma reserva é cancelada.
type ReservationCancelledEvent struct {
	ReservationID string `json:"reservationId"`
	NumeroCabines int    `json:"numeroCabines"`
	CruiseID      int    `json:"cruiseId"`
	Reason        string `json:"reason"`
}

// PaymentLinkRequest representa a requisição do MS Reserva para o MS Pagamento via REST.
type PaymentLinkRequest struct {
	ReservationID string  `json:"reservationId"`
	Valor         float64 `json:"valor"`
	Customer      string  `json:"customer"`
}

// PaymentLinkResponse representa a resposta do MS Pagamento para o MS Reserva via REST.
type PaymentLinkResponse struct {
	ReservationID string `json:"reservationId"`
	PaymentLink   string `json:"paymentLink"`
}

// WebhookPayload representa a notificação enviada pelo sistema externo para o MS Pagamento.
type WebhookPayload struct {
	TransactionID string  `json:"transactionId"`
	ReservationID string  `json:"reservationId"`
	Status        string  `json:"status"` // "aprovada" ou "recusada"
	Valor         float64 `json:"valor"`
	Comprador     string  `json:"comprador"`
}

// PaymentStatusEvent representa a mensagem publicada nas filas 'pagamento-aprovado' ou 'pagamento-recusado'.
type PaymentStatusEvent struct {
	TransactionID string    `json:"transactionId"`
	ReservationID string    `json:"reservationId"`
	CruiseID      int       `json:"cruiseId"`
	Customer      string    `json:"customer"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	IssueDate     time.Time `json:"issueDate"`
}

// TicketGeneratedEvent representa a mensagem publicada na fila 'bilhete-gerado'.
type TicketGeneratedEvent struct {
	TicketID      string    `json:"ticketId"`
	ReservationID string    `json:"reservationId"`
	Customer      string    `json:"customer"`
	CruiseID      int       `json:"cruiseId"`
	Status        string    `json:"status"`
	Message       string    `json:"message"`
	IssuedAt      time.Time `json:"issuedAt"`
}

// PromotionEvent representa uma mensagem publicada na fila 'promocoes'.
type PromotionEvent struct {
	ID           string    `json:"id,omitempty"`
	NomePromocao string    `json:"nomePromocao"`
	CruiseID     int       `json:"cruiseId"`
	Descricao    string    `json:"descricao,omitempty"`
	Timestamp    time.Time `json:"timestamp,omitempty"`
}

// InterestNotification é usada para registrar ou cancelar interesse em promoções.
type InterestNotification struct {
	ClientID   string `json:"clientId"`
	Interested bool   `json:"interested"` // true para registrar, false para cancelar
}
