package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"reserva-cruzeiros/model"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

type Cruzeiro struct {
	ID   int    `json:"id"`
	Nome string `json:"nome"`
}

var listaDeCruzeiros []Cruzeiro

const (
	exchangePromocoes = "promocoes"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func loadCruzeirosData(filePath string) {
	data, err := ioutil.ReadFile(filePath)
	failOnError(err, "Falha ao ler arquivo de cruzeiros")
	err = json.Unmarshal(data, &listaDeCruzeiros)
	failOnError(err, "Falha ao decodificar JSON de cruzeiros")
	log.Printf("%d cruzeiros carregados para promoções.", len(listaDeCruzeiros))
}

func main() {
	log.Println("Iniciando MS Marketing...")
	rand.Seed(time.Now().UnixNano())

	cruzeirosFilePath := os.Getenv("CRUZEIROS_JSON_PATH")
	if cruzeirosFilePath == "" {
		cruzeirosFilePath = "data/cruzeiros.json"
	}
	loadCruzeirosData(cruzeirosFilePath)
	if len(listaDeCruzeiros) == 0 {
		log.Fatal("Nenhum cruzeiro carregado. Encerrando.")
	}

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

	err = ch.ExchangeDeclare(exchangePromocoes, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange de promoções")

	ticker := time.NewTicker(20 * time.Second) // Publica uma promoção a cada 20 segundos
	defer ticker.Stop()

	log.Println("Publicador de promoções iniciado.")

	for range ticker.C {
		cruzeiro := listaDeCruzeiros[rand.Intn(len(listaDeCruzeiros))]
		desconto := 10 + rand.Intn(21) // Desconto entre 10% e 30%

		promo := model.PromotionEvent{
			ID:           uuid.New().String(),
			NomePromocao: fmt.Sprintf("%d%% OFF no cruzeiro %s!", desconto, cruzeiro.Nome),
			CruiseID:     cruzeiro.ID,
			Descricao:    fmt.Sprintf("Aproveite esta oferta por tempo limitado para uma viagem inesquecível!"),
			Timestamp:    time.Now(),
		}

		body, _ := json.Marshal(promo)
		err = ch.Publish(exchangePromocoes, "", false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})
		if err != nil {
			log.Printf("Erro ao publicar promoção: %s", err)
		} else {
			log.Printf("Promoção publicada: %s", promo.NomePromocao)
		}
	}
}
