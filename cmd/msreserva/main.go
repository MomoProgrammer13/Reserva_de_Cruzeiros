package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/streadway/amqp"
)

type Message struct {
	Name string `json:"name"`
}

type Response struct {
	Greeting string `json:"greeting"`
}

var conn *amqp.Connection
var ch *amqp.Channel
var responseQueue amqp.Queue
var responses = make(map[string]chan Response)

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

	// Fila para envio de mensagens
	_, err = ch.QueueDeclare(
		"hello_queue", // name
		false,         // durable
		false,         // delete when unused
		false,         // exclusive
		false,         // no-wait
		nil,           // arguments
	)
	failOnError(err, "Falha ao declarar fila")

	// Fila para receber respostas
	responseQueue, err = ch.QueueDeclare(
		"response_queue", // name
		false,            // durable
		false,            // delete when unused
		false,            // exclusive
		false,            // no-wait
		nil,              // arguments
	)
	failOnError(err, "Falha ao declarar fila de resposta")

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
			var response Response
			err := json.Unmarshal(d.Body, &response)
			if err != nil {
				log.Printf("Erro ao decodificar resposta: %s", err)
				continue
			}

			corrID := d.CorrelationId
			if ch, ok := responses[corrID]; ok {
				ch <- response
				delete(responses, corrID)
			}
		}
	}()
}

func helloHandler(c echo.Context) error {
	name := c.Param("name")

	// Criar ID único para a correlação
	corrID := fmt.Sprintf("%d", time.Now().UnixNano())

	// Criar canal para receber resposta
	responseCh := make(chan Response)
	responses[corrID] = responseCh

	// Preparar mensagem
	message := Message{Name: name}
	body, err := json.Marshal(message)
	if err != nil {
		return c.String(http.StatusInternalServerError, "Erro ao processar requisição")
	}

	// Publicar mensagem
	err = ch.Publish(
		"",            // exchange
		"hello_queue", // routing key
		false,         // mandatory
		false,         // immediate
		amqp.Publishing{
			ContentType:   "application/json",
			CorrelationId: corrID,
			ReplyTo:       responseQueue.Name,
			Body:          body,
		})
	if err != nil {
		return c.String(http.StatusInternalServerError, "Erro ao enviar mensagem")
	}

	// Aguardar resposta com timeout
	select {
	case response := <-responseCh:
		return c.JSON(http.StatusOK, response)
	case <-time.After(5 * time.Second):
		delete(responses, corrID)
		return c.String(http.StatusGatewayTimeout, "Timeout ao aguardar resposta")
	}
}

func main() {
	setupRabbitMQ()
	defer conn.Close()
	defer ch.Close()

	e := echo.New()
	e.GET("/hello/:name", helloHandler)

	fmt.Println("Servidor iniciado na porta 8080")
	e.Logger.Fatal(e.Start(":8080"))
}
