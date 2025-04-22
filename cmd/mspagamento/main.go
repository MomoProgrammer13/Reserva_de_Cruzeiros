package main

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/streadway/amqp"
)

type Message struct {
	Name string `json:"name"`
}

type Response struct {
	Greeting string `json:"greeting"`
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
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

	q, err := ch.QueueDeclare(
		"hello_queue", // name
		false,         // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	failOnError(err, "Falha ao declarar fila")

	msgs, err := ch.Consume(
		q.Name, // queue
		"",     // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	failOnError(err, "Falha ao registrar consumidor")

	forever := make(chan bool)

	go func() {
		for d := range msgs {
			var message Message
			err := json.Unmarshal(d.Body, &message)
			if err != nil {
				log.Printf("Erro ao decodificar mensagem: %s", err)
				continue
			}

			log.Printf("Recebida mensagem: %+v", message)
			
			// Preparar resposta
			greeting := fmt.Sprintf("Olá %s, como você está?", message.Name)
			response := Response{Greeting: greeting}
			
			responseBody, err := json.Marshal(response)
			if err != nil {
				log.Printf("Erro ao codificar resposta: %s", err)
				continue
			}

			// Publicar resposta
			err = ch.Publish(
				"",        // exchange
				d.ReplyTo, // routing key
				false,     // mandatory
				false,     // immediate
				amqp.Publishing{
					ContentType:   "application/json",
					CorrelationId: d.CorrelationId,
					Body:          responseBody,
				})
			if err != nil {
				log.Printf("Erro ao enviar resposta: %s", err)
			}
		}
	}()

	log.Printf("Microserviço de pagamento iniciado. Para sair pressione CTRL+C")
	<-forever
}
