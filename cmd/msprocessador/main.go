package main

import (
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
	"reserva-cruzeiros/cmd/msreserva/models"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

// Simulação de processamento de reserva
func processReservation(request models.ReservationRequest) models.ReservationResponse {
	// Em um sistema real, você verificaria disponibilidade e faria outras validações

	// Validar dados básicos
	if request.Passengers <= 0 {
		return models.ReservationResponse{
			ReservationID: "",
			Status:        "failed",
			Message:       "Número de passageiros inválido",
			CreatedAt:     time.Now(),
		}
	}

	// Simular processamento
	time.Sleep(500 * time.Millisecond)

	return models.ReservationResponse{
		ReservationID: uuid.New().String(),
		Status:        "pending_payment",
		Message:       "Reserva criada com sucesso. Aguardando pagamento.",
		CreatedAt:     time.Now(),
	}
}

func main() {
	// Tenta conectar ao RabbitMQ com retry
	var conn *amqp.Connection
	var err error

	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial("amqp://guest:guest@rabbitmq:5672/")
		if err == nil {
			break
		}
		log.Printf("Falha ao conectar ao RabbitMQ, tentando novamente em 5 segundos...")
		time.Sleep(5 * time.Second)
	}
	failOnError(err, "Falha ao conectar ao RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Falha ao abrir canal")
	defer ch.Close()

	// Configurar prefetch para garantir distribuição justa de trabalho
	err = ch.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	failOnError(err, "Falha ao configurar QoS")

	// Declarar fila de reservas
	q, err := ch.QueueDeclare(
		"reservation_queue", // name
		true,                // durable
		false,               // delete when unused
		false,               // exclusive
		false,               // no-wait
		nil,                 // arguments
	)
	failOnError(err, "Falha ao declarar fila de reservas")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		false,  // auto-ack (mudado para false para confirmar manualmente)
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Falha ao registrar consumidor")

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			var reservationRequest models.ReservationRequest
			err := json.Unmarshal(d.Body, &reservationRequest)
			if err != nil {
				log.Printf("Erro ao decodificar mensagem de reserva: %s", err)
				d.Nack(false, false) // Rejeitar mensagem sem requeue
				continue
			}

			log.Printf("Processando reserva para cruzeiro: %s, cliente: %s, passageiros: %d",
				reservationRequest.CruiseID, reservationRequest.CustomerID, reservationRequest.Passengers)

			// Processar reserva
			response := processReservation(reservationRequest)

			responseBody, err := json.Marshal(response)
			if err != nil {
				log.Printf("Erro ao codificar resposta: %s", err)
				d.Nack(false, true) // Rejeitar mensagem com requeue
				continue
			}

			// Publicar resposta
			err = ch.Publish(
				"",        // exchange
				d.ReplyTo, // routing key
				false,     // mandatory
				false,     // immediate
				amqp.Publishing{
					DeliveryMode:  amqp.Persistent,
					ContentType:   "application/json",
					Type:          "reservation",
					CorrelationId: d.CorrelationId,
					Body:          responseBody,
				})
			if err != nil {
				log.Printf("Erro ao enviar resposta: %s", err)
				d.Nack(false, true) // Rejeitar mensagem com requeue
			} else {
				d.Ack(false) // Confirmar processamento bem-sucedido
				log.Printf("Resposta de reserva enviada, ID: %s, status: %s",
					response.ReservationID, response.Status)
			}
		}
	}()

	log.Printf("Microserviço de processamento de reservas iniciado. Para sair pressione CTRL+C")
	<-forever
}
