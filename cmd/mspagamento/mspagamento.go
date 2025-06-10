package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"reserva-cruzeiros/model"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/streadway/amqp"
)

var amqpChannel *amqp.Channel

const (
	exchangePagamentoAprovado = "pagamento_aprovado"
	exchangePagamentoRecusado = "pagamento_recusado"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func setupRabbitMQ() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitMQURL)
	failOnError(err, "Falha ao conectar ao RabbitMQ")

	ch, err := conn.Channel()
	failOnError(err, "Falha ao abrir canal")
	amqpChannel = ch

	err = ch.ExchangeDeclare(exchangePagamentoAprovado, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange "+exchangePagamentoAprovado)
	err = ch.ExchangeDeclare(exchangePagamentoRecusado, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange "+exchangePagamentoRecusado)

	log.Println("Exchanges de pagamento declaradas.")
}

// Handler da API REST para criar um link de pagamento.
func createPaymentLinkHandler(c echo.Context) error {
	var req model.PaymentLinkRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Dados inválidos"})
	}

	log.Printf("Recebida solicitação de link de pagamento para reserva %s no valor de %.2f", req.ReservationID, req.Valor)

	// Simula a criação de um link de pagamento.
	paymentLink := fmt.Sprintf("https://pagamento.exemplo.com/pay?id=%s", uuid.New().String())

	resp := model.PaymentLinkResponse{
		ReservationID: req.ReservationID,
		PaymentLink:   paymentLink,
	}

	// Simula a chamada do webhook após um tempo.
	go simulateWebhookCall(req)

	return c.JSON(http.StatusOK, resp)
}

// Handler do Webhook que recebe o status do pagamento.
func paymentWebhookHandler(c echo.Context) error {
	var payload model.WebhookPayload
	if err := c.Bind(&payload); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Payload do webhook inválido"})
	}

	log.Printf("Webhook recebido para Reserva %s com status: %s", payload.ReservationID, payload.Status)

	event := model.PaymentStatusEvent{
		TransactionID: payload.TransactionID,
		ReservationID: payload.ReservationID,
		Status:        payload.Status,
		IssueDate:     time.Now(),
		Customer:      payload.Comprador,
	}

	var targetExchange string
	if payload.Status == "aprovada" {
		targetExchange = exchangePagamentoAprovado
		event.Message = "Pagamento aprovado com sucesso"
	} else {
		targetExchange = exchangePagamentoRecusado
		event.Message = "Pagamento foi recusado"
	}

	body, _ := json.Marshal(event)
	err := amqpChannel.Publish(targetExchange, "", false, false, amqp.Publishing{
		ContentType:  "application/json",
		Body:         body,
		DeliveryMode: amqp.Persistent,
	})

	if err != nil {
		log.Printf("Erro ao publicar status de pagamento para reserva %s: %v", payload.ReservationID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Falha ao processar notificação"})
	}

	log.Printf("Notificação de pagamento (%s) para reserva %s publicada na exchange %s", payload.Status, payload.ReservationID, targetExchange)
	return c.JSON(http.StatusOK, map[string]string{"status": "notificação recebida"})
}

// Função para simular o sistema de pagamento externo chamando nosso webhook.
func simulateWebhookCall(req model.PaymentLinkRequest) {
	time.Sleep(time.Duration(5+rand.Intn(5)) * time.Second) // Espera entre 5-10s

	status := "aprovada"
	if rand.Intn(10) < 2 { // 20% de chance de ser recusado
		status = "recusada"
	}

	payload := model.WebhookPayload{
		TransactionID: uuid.New().String(),
		ReservationID: req.ReservationID,
		Status:        status,
		Valor:         req.Valor,
		Comprador:     req.Customer,
	}

	body, _ := json.Marshal(payload)
	webhookURL := os.Getenv("WEBHOOK_URL")
	if webhookURL == "" {
		webhookURL = "http://localhost:8081/webhook/payment-status"
	}

	_, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("SIMULADOR: Falha ao chamar o webhook para reserva %s: %v", req.ReservationID, err)
	} else {
		log.Printf("SIMULADOR: Webhook para reserva %s chamado com status '%s'", req.ReservationID, status)
	}
}

func main() {
	log.Println("Iniciando MS Pagamento...")
	rand.Seed(time.Now().UnixNano())

	setupRabbitMQ()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodPost},
	}))

	e.POST("/create-payment-link", createPaymentLinkHandler)
	e.POST("/webhook/payment-status", paymentWebhookHandler)

	httpPort := os.Getenv("HTTP_PORT_MSPAGAMENTO")
	if httpPort == "" {
		httpPort = "8081"
	}
	log.Printf("Servidor de pagamento HTTP escutando na porta %s", httpPort)
	if err := e.Start(":" + httpPort); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Erro ao iniciar servidor HTTP: %v", err)
	}
}
