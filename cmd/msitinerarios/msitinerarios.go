package main

import (
	"encoding/json"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/streadway/amqp"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reserva-cruzeiros/model"
	"sync"
)

// Cruzeiro define a estrutura de dados para um cruzeiro, incluindo dados dinâmicos.
type Cruzeiro struct {
	ID                 int      `json:"id"`
	Nome               string   `json:"nome"`
	Empresa            string   `json:"empresa"`
	Itinerario         []string `json:"itinerario"`
	PortoEmbarque      string   `json:"portoEmbarque"`
	PortoDesembarque   string   `json:"portoDesembarque"`
	DataEmbarque       string   `json:"dataEmbarque"`
	DataDesembarque    string   `json:"dataDesembarque"`
	CabinesDisponiveis int      `json:"cabinesDisponiveis"`
	ValorCabine        float64  `json:"valorCabine"`
	ImagemURL          string   `json:"imagemURL"`
	DescricaoDetalhada string   `json:"descricaoDetalhada"`
}

var (
	itinerarios      = make(map[int]*Cruzeiro)
	itinerariosMutex sync.RWMutex
)

const (
	exchangeReservaCriada        = "reserva_criada"
	queueConsumoReservaCriada    = "msitinerarios_consumo_reserva_criada"
	exchangeReservaCancelada     = "reserva_cancelada"
	queueConsumoReservaCancelada = "msitinerarios_consumo_reserva_cancelada"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func loadCruzeirosData(filePath string) {
	data, err := ioutil.ReadFile(filePath)
	failOnError(err, "Falha ao ler arquivo de cruzeiros")

	var cruzeirosList []Cruzeiro
	err = json.Unmarshal(data, &cruzeirosList)
	failOnError(err, "Falha ao decodificar JSON de cruzeiros")

	itinerariosMutex.Lock()
	defer itinerariosMutex.Unlock()
	for i := range cruzeirosList {
		itinerarios[cruzeirosList[i].ID] = &cruzeirosList[i]
	}
	log.Printf("%d cruzeiros carregados no mapa.", len(itinerarios))
}

func setupRabbitMQConsumers() {
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}
	conn, err := amqp.Dial(rabbitMQURL)
	failOnError(err, "Falha ao conectar ao RabbitMQ")

	go consume(conn, exchangeReservaCriada, queueConsumoReservaCriada, handleReservaCriada)
	go consume(conn, exchangeReservaCancelada, queueConsumoReservaCancelada, handleReservaCancelada)

	log.Println("Consumidores RabbitMQ para MS Itinerários configurados.")
}

func consume(conn *amqp.Connection, exchangeName, queueName string, handler func(amqp.Delivery)) {
	ch, err := conn.Channel()
	failOnError(err, "Falha ao abrir canal para consumidor")

	err = ch.ExchangeDeclare(exchangeName, "fanout", true, false, false, false, nil)
	failOnError(err, "Falha ao declarar exchange "+exchangeName)

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	failOnError(err, "Falha ao declarar fila "+queueName)

	err = ch.QueueBind(q.Name, "", exchangeName, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao vincular fila %s à exchange %s", q.Name, exchangeName))

	msgs, err := ch.Consume(q.Name, "", false, false, false, false, nil)
	failOnError(err, "Falha ao registrar consumidor")

	log.Printf("Aguardando mensagens na fila %s...", q.Name)
	for d := range msgs {
		log.Printf("Mensagem recebida na fila %s", queueName)
		handler(d)
	}
}

func handleReservaCriada(d amqp.Delivery) {
	var event model.ReservationCreatedEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		log.Printf("Erro ao decodificar evento de reserva criada: %s", err)
		d.Nack(false, false)
		return
	}

	itinerariosMutex.Lock()
	defer itinerariosMutex.Unlock()

	if cruzeiro, ok := itinerarios[event.CruiseID]; ok {
		if cruzeiro.CabinesDisponiveis >= event.NumeroCabines {
			cruzeiro.CabinesDisponiveis -= event.NumeroCabines
			log.Printf("Reserva %s: %d cabines deduzidas do cruzeiro %d. Restam: %d", event.ReservationID, event.NumeroCabines, event.CruiseID, cruzeiro.CabinesDisponiveis)
			d.Ack(false)
		} else {
			log.Printf("Reserva %s: Não há cabines suficientes para o cruzeiro %d. Solicitado: %d, Disponível: %d", event.ReservationID, event.CruiseID, event.NumeroCabines, cruzeiro.CabinesDisponiveis)
			d.Nack(false, false)
		}
	} else {
		log.Printf("Cruzeiro com ID %d não encontrado para a reserva %s", event.CruiseID, event.ReservationID)
		d.Nack(false, false)
	}
}

func handleReservaCancelada(d amqp.Delivery) {
	var event model.ReservationCancelledEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		log.Printf("Erro ao decodificar evento de reserva cancelada: %s", err)
		d.Nack(false, false)
		return
	}

	itinerariosMutex.Lock()
	defer itinerariosMutex.Unlock()

	if cruzeiro, ok := itinerarios[event.CruiseID]; ok {
		cruzeiro.CabinesDisponiveis += event.NumeroCabines
		log.Printf("Reserva %s cancelada: %d cabines retornaram ao estoque do cruzeiro %d. Total agora: %d", event.ReservationID, event.NumeroCabines, event.CruiseID, cruzeiro.CabinesDisponiveis)
		d.Ack(false)
	} else {
		log.Printf("Cruzeiro com ID %d não encontrado para o cancelamento da reserva %s", event.CruiseID, event.ReservationID)
		d.Nack(false, false)
	}
}

func getItinerariesHandler(c echo.Context) error {
	itinerariosMutex.RLock()
	defer itinerariosMutex.RUnlock()

	// Retorna todos os itinerários como uma lista
	var result []*Cruzeiro
	for _, cruzeiro := range itinerarios {
		result = append(result, cruzeiro)
	}

	return c.JSON(http.StatusOK, result)
}

func main() {
	log.Println("Iniciando MS Itinerários...")

	cruzeirosFilePath := os.Getenv("CRUZEIROS_JSON_PATH")
	if cruzeirosFilePath == "" {
		cruzeirosFilePath = "data/cruzeiros.json"
	}
	loadCruzeirosData(cruzeirosFilePath)

	// Inicia os consumidores em uma goroutine para não bloquear o servidor HTTP
	go setupRabbitMQConsumers()

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet},
	}))

	e.GET("/itineraries", getItinerariesHandler)

	httpPort := os.Getenv("HTTP_PORT_MSITINERARIOS")
	if httpPort == "" {
		httpPort = "8082"
	}
	log.Printf("Servidor de itinerários HTTP escutando na porta %s", httpPort)
	if err := e.Start(":" + httpPort); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Erro ao iniciar servidor HTTP: %v", err)
	}
}
