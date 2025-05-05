package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/streadway/amqp"
	"reserva-cruzeiros/cmd/msreserva/models"
)

var conn *amqp.Connection
var ch *amqp.Channel
var responseQueue amqp.Queue
var responses = make(map[string]chan interface{})

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func setupRabbitMQ() {
	var err error

	// Tenta conectar ao RabbitMQ com retry
	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial("amqp://guest:guest@rabbitmq:5672/")
		if err == nil {
			break
		}
		log.Printf("Falha ao conectar ao RabbitMQ, tentando novamente em 5 segundos...")
		time.Sleep(5 * time.Second)
	}
	failOnError(err, "Falha ao conectar ao RabbitMQ")

	ch, err = conn.Channel()
	failOnError(err, "Falha ao abrir canal")

	// Fila para envio de mensagens de reserva
	_, err = ch.QueueDeclare(
		"reservation_queue", // name
		true,                // durable
		false,               // delete when unused
		false,               // exclusive
		false,               // no-wait
		nil,                 // arguments
	)
	failOnError(err, "Falha ao declarar fila de reservas")

	// Fila para envio de mensagens de pagamento
	_, err = ch.QueueDeclare(
		"payment_queue", // name
		true,            // durable
		false,           // delete when unused
		false,           // exclusive
		false,           // no-wait
		nil,             // arguments
	)
	failOnError(err, "Falha ao declarar fila de pagamentos")

	// Fila para receber respostas
	responseQueue, err = ch.QueueDeclare(
		"response_queue", // name
		true,             // durable
		false,            // delete when unused
		false,            // exclusive
		false,            // no-wait
		nil,              // arguments
	)
	failOnError(err, "Falha ao declarar fila de resposta")

	// Configurar prefetch para garantir distribuição justa de trabalho
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	failOnError(err, "Falha ao configurar QoS")

	// Consumir respostas
	msgs, err := ch.Consume(
		responseQueue.Name, // queue
		"",                 // consumer
		true,               // auto-ack
		false,              // exclusive
		false,              // no-local
		false,              // no-wait
		nil,                // args
	)
	failOnError(err, "Falha ao registrar consumidor")

	go func() {
		for d := range msgs {
			corrID := d.CorrelationId
			if ch, ok := responses[corrID]; ok {
				// Determinar o tipo de resposta com base no tipo de conteúdo
				switch d.Type {
				case "payment":
					var response models.PaymentResponse
					err := json.Unmarshal(d.Body, &response)
					if err != nil {
						log.Printf("Erro ao decodificar resposta de pagamento: %s", err)
						continue
					}
					ch <- response
				case "reservation":
					var response models.ReservationResponse
					err := json.Unmarshal(d.Body, &response)
					if err != nil {
						log.Printf("Erro ao decodificar resposta de reserva: %s", err)
						continue
					}
					ch <- response
				default:
					log.Printf("Tipo de mensagem desconhecido: %s", d.Type)
				}
				delete(responses, corrID)
			}
		}
	}()
}

func createReservationHandler(c echo.Context) error {
	var request models.ReservationRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dados inválidos para reserva",
		})
	}

	// Validar dados da reserva
	if request.CruiseID == "" || request.CustomerID == "" || request.Passengers <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dados incompletos para reserva",
		})
	}

	// Criar ID único para a correlação
	corrID := uuid.New().String()

	// Criar canal para receber resposta
	responseCh := make(chan interface{})
	responses[corrID] = responseCh

	// Preparar mensagem
	body, err := json.Marshal(request)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao processar requisição",
		})
	}

	// Publicar mensagem
	err = ch.Publish(
		"",                  // exchange
		"reservation_queue", // routing key
		false,               // mandatory
		false,               // immediate
		amqp.Publishing{
			DeliveryMode:  amqp.Persistent,
			ContentType:   "application/json",
			Type:          "reservation",
			CorrelationId: corrID,
			ReplyTo:       responseQueue.Name,
			Body:          body,
		})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao enviar mensagem",
		})
	}

	// Aguardar resposta com timeout
	select {
	case response := <-responseCh:
		return c.JSON(http.StatusOK, response)
	case <-time.After(10 * time.Second):
		delete(responses, corrID)
		return c.JSON(http.StatusGatewayTimeout, map[string]string{
			"error": "Timeout ao aguardar resposta do serviço de reservas",
		})
	}
}

func processPaymentHandler(c echo.Context) error {
	var request models.PaymentRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dados inválidos para pagamento",
		})
	}

	// Validar dados de pagamento
	if request.ReservationID == "" || request.Amount <= 0 || request.CardNumber == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dados incompletos para pagamento",
		})
	}

	// Criar ID único para a correlação
	corrID := uuid.New().String()

	// Criar canal para receber resposta
	responseCh := make(chan interface{})
	responses[corrID] = responseCh

	// Preparar mensagem
	body, err := json.Marshal(request)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao processar requisição",
		})
	}

	// Publicar mensagem
	err = ch.Publish(
		"",              // exchange
		"payment_queue", // routing key
		false,           // mandatory
		false,           // immediate
		amqp.Publishing{
			DeliveryMode:  amqp.Persistent,
			ContentType:   "application/json",
			Type:          "payment",
			CorrelationId: corrID,
			ReplyTo:       responseQueue.Name,
			Body:          body,
		})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao enviar mensagem",
		})
	}

	// Aguardar resposta com timeout
	select {
	case response := <-responseCh:
		return c.JSON(http.StatusOK, response)
	case <-time.After(10 * time.Second):
		delete(responses, corrID)
		return c.JSON(http.StatusGatewayTimeout, map[string]string{
			"error": "Timeout ao aguardar resposta do serviço de pagamento",
		})
	}
}

func getReservationStatusHandler(c echo.Context) error {
	reservationID := c.Param("id")

	// Em um sistema real, você consultaria um banco de dados aqui
	// Para este exemplo, retornaremos uma resposta simulada

	return c.JSON(http.StatusOK, models.ReservationResponse{
		ReservationID: reservationID,
		Status:        "confirmed",
		Message:       "Reserva confirmada com sucesso",
		CreatedAt:     time.Now().Add(-24 * time.Hour),
	})
}

func main() {
	setupRabbitMQ()
	defer conn.Close()
	defer ch.Close()

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Rotas
	e.POST("/reservations", createReservationHandler)
	e.POST("/payments", processPaymentHandler)
	e.GET("/reservations/:id", getReservationStatusHandler)

	// Rota de saúde
	e.GET("/health", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{
			"status":  "UP",
			"service": "msreserva",
		})
	})

	fmt.Println("Servidor de reservas iniciado na porta 8080")
	e.Logger.Fatal(e.Start(":8080"))
}
