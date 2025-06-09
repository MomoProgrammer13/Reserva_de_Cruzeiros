package main

import (
	"encoding/json"
	_ "fmt"
	"log"
	"os"
	"reserva-cruzeiros/model"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

const (
	exchangePagamentoAprovado = "pagamento_aprovado"
	queueConsumoPagamento     = "msbilhete_consumo_pagamento_aprovado"
	exchangeBilheteGerado     = "bilhete_gerado"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func main() {
	log.Println("Iniciando MS Bilhete...")
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitMQURL)
	failOnError(err, "Falha ao conectar ao RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Falha ao abrir canal")
	defer ch.Close()

	// Exchange de consumo
	err = ch.ExchangeDeclare(exchangePagamentoAprovado, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange de consumo")

	q, err := ch.QueueDeclare(queueConsumoPagamento, true, false, false, false, nil)
	failOnError(err, "Falha ao declarar fila")

	err = ch.QueueBind(q.Name, "", exchangePagamentoAprovado, false, nil)
	failOnError(err, "Falha ao vincular fila")

	// Exchange de publicação
	err = ch.ExchangeDeclare(exchangeBilheteGerado, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange de publicação")

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	failOnError(err, "Falha ao registrar consumidor")

	forever := make(chan bool)
	go func() {
		for d := range msgs {
			log.Printf("Recebido pagamento aprovado. Gerando bilhete...")
			var event model.PaymentStatusEvent
			if err := json.Unmarshal(d.Body, &event); err != nil {
				log.Printf("Erro ao decodificar evento de pagamento: %s", err)
				d.Nack(false, false)
				continue
			}

			ticketEvent := model.TicketGeneratedEvent{
				TicketID:      uuid.New().String(),
				ReservationID: event.ReservationID,
				Customer:      event.Customer,
				CruiseID:      event.CruiseID,
				Status:        "Emitido",
				Message:       "Seu bilhete foi emitido com sucesso!",
				IssuedAt:      time.Now(),
			}

			body, _ := json.Marshal(ticketEvent)
			err = ch.Publish(exchangeBilheteGerado, "", false, false, amqp.Publishing{
				ContentType: "application/json",
				Body:        body,
			})

			if err != nil {
				log.Printf("Erro ao publicar evento de bilhete gerado: %s", err)
				d.Nack(false, true)
			} else {
				log.Printf("Bilhete para reserva %s publicado com sucesso.", ticketEvent.ReservationID)
				d.Ack(false)
			}
		}
	}()

	log.Printf(" [*] Aguardando por pagamentos aprovados. Para sair pressione CTRL+C")
	<-forever
}
