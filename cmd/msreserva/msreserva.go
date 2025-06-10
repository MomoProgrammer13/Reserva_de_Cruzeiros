package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"reserva-cruzeiros/model"
	"sync"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/streadway/amqp"
)

type SseClient struct {
	channel           chan string
	interestedInPromo bool
}

var (
	clients                = make(map[string]*SseClient)
	clientsMu              sync.RWMutex
	activeReservations     = make(map[string]model.ReservationCreatedEvent)
	reservationsMutex      sync.Mutex
	reservationToClientMap = make(map[string]string)
	mapMutex               sync.RWMutex
)

var amqpChannel *amqp.Channel

const (
	exchangeReservaCriada         = "reserva_criada"
	exchangeReservaCancelada      = "reserva_cancelada"
	exchangePagamentoAprovado     = "pagamento_aprovado"
	exchangePagamentoRecusado     = "pagamento_recusado"
	exchangeBilheteGerado         = "bilhete_gerado"
	exchangePromocoes             = "promocoes"
	queueConsumoPagamentoAprovado = "msreserva_consumo_pagamento_aprovado"
	queueConsumoPagamentoRecusado = "msreserva_consumo_pagamento_recusado"
	queueConsumoBilheteGerado     = "msreserva_consumo_bilhete_gerado"
	queueConsumoPromocoes         = "msreserva_consumo_promocoes"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func formatSseMessage(event, data string) string {
	return fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
}

func broadcastToInterested(message string) {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for id, client := range clients {
		if client.interestedInPromo {
			select {
			case client.channel <- message:
			default:
				log.Printf("Canal do cliente %s (interessado em promo) bloqueado.", id)
			}
		}
	}
}

func notifyClient(clientID, message string) {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	if client, ok := clients[clientID]; ok {
		select {
		case client.channel <- message:
		default:
			log.Printf("Canal do cliente %s bloqueado.", clientID)
		}
	} else {
		log.Printf("Cliente SSE com ID %s não encontrado para notificação.", clientID)
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

	ch.ExchangeDeclare(exchangeReservaCriada, "fanout", true, false, false, false, nil)
	ch.ExchangeDeclare(exchangeReservaCancelada, "fanout", true, false, false, false, nil)

	go consume(conn, exchangePagamentoAprovado, queueConsumoPagamentoAprovado, handlePaymentStatus)
	go consume(conn, exchangePagamentoRecusado, queueConsumoPagamentoRecusado, handlePaymentStatus)
	go consume(conn, exchangeBilheteGerado, queueConsumoBilheteGerado, handleTicketGenerated)
	go consume(conn, exchangePromocoes, queueConsumoPromocoes, handlePromotion)
	log.Println("Setup RabbitMQ para MS Reserva concluído.")
}

// CORRIGIDO: Removido 'defer ch.Close()' para manter os consumidores a correr.
func consume(conn *amqp.Connection, exchangeName, queueName string, handler func(amqp.Delivery)) {
	ch, err := conn.Channel()
	failOnError(err, fmt.Sprintf("Falha ao abrir canal para consumidor de %s", queueName))

	err = ch.ExchangeDeclare(exchangeName, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange "+exchangeName)

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	failOnError(err, "Falha ao declarar fila "+queueName)

	err = ch.QueueBind(q.Name, "", exchangeName, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao vincular fila %s à exchange %s", q.Name, exchangeName))

	msgs, err := ch.Consume(q.Name, "", true, false, false, false, nil)
	failOnError(err, "Falha ao registrar consumidor para "+queueName)

	log.Printf("Consumidor iniciado para a fila %s", queueName)
	for d := range msgs {
		handler(d)
	}
	log.Printf("Consumidor para a fila %s a terminar.", queueName)
}

func handlePaymentStatus(d amqp.Delivery) {
	var event model.PaymentStatusEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		log.Printf("Erro ao decodificar evento de pagamento: %v", err)
		return
	}
	log.Printf("Evento de status de pagamento recebido para ReservationID: %s", event.ReservationID)

	mapMutex.RLock()
	clientID, ok := reservationToClientMap[event.ReservationID]
	mapMutex.RUnlock()

	if !ok {
		log.Printf("AVISO: ClientID para a reserva %s não encontrado. A notificação não pode ser enviada.", event.ReservationID)
		return
	}

	sseEvent := "pagamento_aprovado"
	if event.Status != "aprovada" {
		sseEvent = "pagamento_recusado"
	}
	notifyClient(clientID, formatSseMessage(sseEvent, string(d.Body)))

	if event.Status == "recusada" {
		cancelDueToPaymentFailure(event.ReservationID)
	}
}

func handleTicketGenerated(d amqp.Delivery) {
	var event model.TicketGeneratedEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		log.Printf("Erro ao decodificar evento de bilhete: %v", err)
		return
	}
	log.Printf("Evento de bilhete gerado recebido para ReservationID: %s", event.ReservationID)

	mapMutex.RLock()
	clientID, ok := reservationToClientMap[event.ReservationID]
	mapMutex.RUnlock()

	if !ok {
		log.Printf("AVISO: ClientID para a reserva %s não encontrado. A notificação de bilhete não pode ser enviada.", event.ReservationID)
		return
	}

	notifyClient(clientID, formatSseMessage("bilhete_gerado", string(d.Body)))
}

func handlePromotion(d amqp.Delivery) {
	log.Printf("Evento de promoção recebido.")
	broadcastToInterested(formatSseMessage("promocao", string(d.Body)))
}

func getItinerariesHandler(c echo.Context) error {
	msItinerariosURL := os.Getenv("MS_ITINERARIOS_URL")
	if msItinerariosURL == "" {
		msItinerariosURL = "http://localhost:8082/itineraries"
	}

	resp, err := http.Get(msItinerariosURL)
	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "MS Itinerários indisponível"})
	}
	defer resp.Body.Close()

	var itineraries []interface{}
	json.NewDecoder(resp.Body).Decode(&itineraries)
	return c.JSON(http.StatusOK, itineraries)
}

func createReservationHandler(c echo.Context) error {
	var req model.ReservationRequestWithClient
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Dados de reserva inválidos: " + err.Error()})
	}

	// CORRIGIDO: Adicionada verificação para garantir que o ClientID não está vazio.
	if req.ClientID == "" {
		log.Println("ERRO CRÍTICO: Pedido de reserva recebido sem ClientID.")
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ClientID é obrigatório"})
	}

	event := model.ReservationCreatedEvent{
		ReservationID:      uuid.New().String(),
		ReservationRequest: req.ReservationRequest,
	}
	log.Printf("Iniciando criação de reserva ID: %s para ClientID: %s", event.ReservationID, req.ClientID)

	mapMutex.Lock()
	reservationToClientMap[event.ReservationID] = req.ClientID
	mapMutex.Unlock()
	log.Printf("Mapeamento criado: ReservationID %s -> ClientID %s", event.ReservationID, req.ClientID)

	reservationsMutex.Lock()
	activeReservations[event.ReservationID] = event
	reservationsMutex.Unlock()

	body, _ := json.Marshal(event)
	amqpChannel.Publish(exchangeReservaCriada, "", false, false, amqp.Publishing{
		ContentType: "application/json", Body: body,
	})
	log.Printf("Evento de reserva %s publicado.", event.ReservationID)

	paymentReq := model.PaymentLinkRequest{
		ReservationID: event.ReservationID,
		Valor:         req.ValorTotal,
		Customer:      req.Customer,
	}
	paymentBody, _ := json.Marshal(paymentReq)
	msPagamentoURL := os.Getenv("MS_PAGAMENTO_URL")
	if msPagamentoURL == "" {
		msPagamentoURL = "http://localhost:8081/create-payment-link"
	}

	resp, err := http.Post(msPagamentoURL, "application/json", bytes.NewBuffer(paymentBody))
	if err != nil || resp.StatusCode != http.StatusOK {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"error": "Falha ao contatar serviço de pagamento"})
	}
	defer resp.Body.Close()

	var paymentLinkResp model.PaymentLinkResponse
	json.NewDecoder(resp.Body).Decode(&paymentLinkResp)

	return c.JSON(http.StatusCreated, paymentLinkResp)
}

func cancelReservationHandler(c echo.Context) error {
	reservationID := c.Param("reservationId")
	if reservationID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ID da reserva é obrigatório"})
	}

	reservationsMutex.Lock()
	res, ok := activeReservations[reservationID]
	if !ok {
		reservationsMutex.Unlock()
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Reserva não encontrada ou já finalizada"})
	}
	delete(activeReservations, reservationID)
	reservationsMutex.Unlock()

	mapMutex.Lock()
	delete(reservationToClientMap, reservationID)
	mapMutex.Unlock()

	event := model.ReservationCancelledEvent{
		ReservationID: reservationID,
		CruiseID:      res.CruiseID,
		NumeroCabines: res.NumeroCabines,
		Reason:        "Cancelado pelo cliente via endpoint",
	}

	body, _ := json.Marshal(event)
	amqpChannel.Publish(exchangeReservaCancelada, "", false, false, amqp.Publishing{
		ContentType: "application/json", Body: body,
	})

	log.Printf("Solicitação de cancelamento para reserva %s enviada.", reservationID)
	return c.JSON(http.StatusOK, map[string]string{"message": "Solicitação de cancelamento processada"})
}

func cancelDueToPaymentFailure(reservationID string) {
	reservationsMutex.Lock()
	res, ok := activeReservations[reservationID]
	if !ok {
		reservationsMutex.Unlock()
		return
	}
	delete(activeReservations, reservationID)
	reservationsMutex.Unlock()

	mapMutex.Lock()
	delete(reservationToClientMap, reservationID)
	mapMutex.Unlock()

	event := model.ReservationCancelledEvent{
		ReservationID: reservationID,
		CruiseID:      res.CruiseID,
		NumeroCabines: res.NumeroCabines,
		Reason:        "Pagamento recusado",
	}

	body, _ := json.Marshal(event)
	amqpChannel.Publish(exchangeReservaCancelada, "", false, false, amqp.Publishing{
		ContentType: "application/json", Body: body,
	})
	log.Printf("Cancelamento por falha de pagamento para reserva %s publicado.", reservationID)
}

func sseNotificationsHandler(c echo.Context) error {
	clientID := c.Param("clientId")
	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	client := &SseClient{channel: make(chan string, 10)}

	clientsMu.Lock()
	clients[clientID] = client
	clientsMu.Unlock()
	log.Printf("Cliente SSE %s conectado.", clientID)

	defer func() {
		clientsMu.Lock()
		delete(clients, clientID)
		clientsMu.Unlock()
		log.Printf("Cliente SSE %s desconectado.", clientID)
	}()

	fmt.Fprintf(c.Response().Writer, formatSseMessage("connection_ready", `{"clientId": "`+clientID+`"}`))
	c.Response().Flush()

	for {
		select {
		case msg := <-client.channel:
			if _, err := fmt.Fprint(c.Response().Writer, msg); err != nil {
				return nil
			}
			c.Response().Flush()
		case <-c.Request().Context().Done():
			return nil
		}
	}
}

func updateInterestHandler(c echo.Context) error {
	var req model.InterestNotification
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Payload inválido"})
	}

	clientsMu.Lock()
	defer clientsMu.Unlock()

	if client, ok := clients[req.ClientID]; ok {
		client.interestedInPromo = req.Interested
		status := "ativado"
		if !req.Interested {
			status = "desativado"
		}
		log.Printf("Interesse em promoções %s para o cliente %s", status, req.ClientID)
		return c.JSON(http.StatusOK, map[string]interface{}{"clientId": req.ClientID, "interest": status})
	}
	return c.JSON(http.StatusNotFound, map[string]string{"error": "Cliente não encontrado"})
}

func main() {
	log.Println("Iniciando MS Reserva...")
	setupRabbitMQ()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))

	e.GET("/itineraries", getItinerariesHandler)
	e.POST("/reservations", createReservationHandler)
	e.DELETE("/reservations/:reservationId", cancelReservationHandler)
	e.GET("/notifications/:clientId", sseNotificationsHandler)
	e.POST("/notifications/interest", updateInterestHandler)

	httpPort := os.Getenv("HTTP_PORT_MSRESERVA")
	if httpPort == "" {
		httpPort = "8080"
	}
	log.Printf("Servidor de reservas HTTP escutando na porta %s", httpPort)
	if err := e.Start(":" + httpPort); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Erro ao iniciar servidor HTTP: %v", err)
	}
}
