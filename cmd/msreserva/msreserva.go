package main

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"reserva-cruzeiros/model" // Certifique-se que este path está correto
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/streadway/amqp"
)

// Estrutura para representar os dados de um cruzeiro, correspondendo ao cruzeiros.json
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
	// Chave privada deste microsserviço (MSReserva)
	msReservaPrivateKey *rsa.PrivateKey
	// Chaves públicas dos outros microsserviços dos quais MSReserva consome mensagens
	msPagamentoPublicKey *rsa.PublicKey
	msBilhetePublicKey   *rsa.PublicKey

	listaDeCruzeiros []Cruzeiro
)

var (
	responses      = make(map[string]chan model.BilheteResponse)
	paymentResults = make(map[string]chan model.PaymentResponse)
	mu             sync.Mutex
)

var amqpConnection *amqp.Connection
var amqpChannel *amqp.Channel

const (
	exchangeReservaCriada     = "reserva_criada"
	exchangePagamentoAprovado = "pagamento_aprovado"
	exchangePagamentoRecusado = "pagamento_recusado"
	exchangeBilheteGerado     = "bilhete_gerado"
)

const (
	filaConsumoPagamentosAprovados = "msreserva_consumo_pagamento_aprovado"
	filaConsumoPagamentosRecusados = "msreserva_consumo_pagamento_recusado"
	filaConsumoBilhetesGerados     = "msreserva_consumo_bilhete_gerado"
)

const (
	senderIDMSReserva   = "msreserva"
	senderIDMSPagamento = "mspagamento"
	senderIDMSBilhete   = "msbilhete"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func loadCruzeirosData(filePath string) error {
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("falha ao ler arquivo de cruzeiros %s: %w", filePath, err)
	}
	err = json.Unmarshal(data, &listaDeCruzeiros)
	if err != nil {
		return fmt.Errorf("falha ao decodificar JSON de cruzeiros: %w", err)
	}
	log.Printf("%d cruzeiros carregados de %s", len(listaDeCruzeiros), filePath)
	return nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	privateKeyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler chave privada de %s: %w", path, err)
	}
	block, _ := pem.Decode(privateKeyData)
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave privada de %s", path)
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear chave privada PKCS1 de %s: %w", path, err)
	}
	return privateKey, nil
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	publicKeyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler chave pública de %s: %w", path, err)
	}
	block, _ := pem.Decode(publicKeyData)
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave pública de %s", path)
	}
	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear chave pública PKIX de %s: %w", path, err)
	}
	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("chave pública de %s não é do tipo RSA", path)
	}
	return publicKey, nil
}

func loadCertificates() {
	var err error
	// Carregar chave privada do MSReserva
	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH_MSRESERVA")
	if privateKeyPath == "" {
		privateKeyPath = "certificates/msreserva/private_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			privateKeyPath = "/app/certs/msreserva/private_key.pem"
		}
	}
	msReservaPrivateKey, err = loadPrivateKey(privateKeyPath)
	failOnError(err, fmt.Sprintf("MSReserva: Não foi possível carregar a chave privada de %s", privateKeyPath))
	log.Println("MSReserva: Chave privada carregada com sucesso.")

	// Carregar chave pública do MSPagamento (para verificar mensagens de pagamento_aprovado/recusado)
	msPagamentoPublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSPAGAMENTO")
	if msPagamentoPublicKeyPath == "" {
		msPagamentoPublicKeyPath = "certificates/mspagamento/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msPagamentoPublicKeyPath = "/app/certs/mspagamento/public_key.pem"
		}
	}
	msPagamentoPublicKey, err = loadPublicKey(msPagamentoPublicKeyPath)
	failOnError(err, fmt.Sprintf("MSReserva: Não foi possível carregar a chave pública do MSPagamento de %s", msPagamentoPublicKeyPath))
	log.Println("MSReserva: Chave pública do MSPagamento carregada com sucesso.")

	// Carregar chave pública do MSBilhete (para verificar mensagens de bilhete_gerado)
	msBilhetePublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSBILHETE")
	if msBilhetePublicKeyPath == "" {
		msBilhetePublicKeyPath = "certificates/msbilhete/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msBilhetePublicKeyPath = "/app/certs/msbilhete/public_key.pem"
		}
	}
	msBilhetePublicKey, err = loadPublicKey(msBilhetePublicKeyPath)
	failOnError(err, fmt.Sprintf("MSReserva: Não foi possível carregar a chave pública do MSBilhete de %s", msBilhetePublicKeyPath))
	log.Println("MSReserva: Chave pública do MSBilhete carregada com sucesso.")
}

// signMessage assina a mensagem usando a chave privada fornecida.
func signMessage(message []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hashed := sha256.Sum256(message)
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, fmt.Errorf("erro ao assinar mensagem: %w", err)
	}
	return signature, nil
}

// verifySignature verifica a assinatura da mensagem usando a chave pública fornecida.
func verifySignature(message, signature []byte, publicKey *rsa.PublicKey) error {
	hashed := sha256.Sum256(message)
	err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hashed[:], signature)
	if err != nil {
		return fmt.Errorf("falha na verificação da assinatura: %w", err)
	}
	return nil
}

func setupRabbitMQ(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()
	log.Println("Iniciando configuração RabbitMQ para msreserva...")

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}

	var err error
	maxRetries := 10
	retryDelay := 5 * time.Second

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			log.Println("Contexto de setup RabbitMQ cancelado.")
			return
		default:
		}

		amqpConnection, err = amqp.Dial(rabbitMQURL)
		if err == nil {
			log.Println("msreserva: Conectado ao RabbitMQ com sucesso.")
			amqpChannel, err = amqpConnection.Channel()
			failOnError(err, "msreserva: Falha ao abrir canal principal para publicações")

			err = amqpChannel.ExchangeDeclare(
				exchangeReservaCriada, "fanout", true, false, false, false, nil)
			failOnError(err, fmt.Sprintf("msreserva: Falha ao declarar exchange '%s'", exchangeReservaCriada))
			log.Printf("msreserva: Exchange '%s' (fanout) declarada.", exchangeReservaCriada)

			wg.Add(3)
			go consumeMessages(ctx, wg, amqpConnection, exchangePagamentoAprovado, filaConsumoPagamentosAprovados, handlePagamentoAprovado, msPagamentoPublicKey, senderIDMSPagamento)
			go consumeMessages(ctx, wg, amqpConnection, exchangePagamentoRecusado, filaConsumoPagamentosRecusados, handlePagamentoRecusado, msPagamentoPublicKey, senderIDMSPagamento)
			go consumeMessages(ctx, wg, amqpConnection, exchangeBilheteGerado, filaConsumoBilhetesGerados, handleBilheteGerado, msBilhetePublicKey, senderIDMSBilhete)

			go func() {
				<-ctx.Done()
				if amqpChannel != nil {
					log.Println("msreserva: Fechando canal AMQP principal...")
					amqpChannel.Close()
				}
				if amqpConnection != nil {
					log.Println("msreserva: Fechando conexão AMQP principal...")
					amqpConnection.Close()
				}
			}()
			return
		}
		log.Printf("msreserva: Falha ao conectar ao RabbitMQ (tentativa %d/%d): %v. Tentando novamente em %v...", i+1, maxRetries, err, retryDelay)
		time.Sleep(retryDelay)
	}
	failOnError(err, "msreserva: Falha ao conectar ao RabbitMQ após múltiplas tentativas")
}

func consumeMessages(ctx context.Context, wg *sync.WaitGroup, conn *amqp.Connection, exchangeName, queueName string, handlerFunc func(msg amqp.Delivery) error, expectedSenderPublicKey *rsa.PublicKey, expectedSenderID string) {
	defer wg.Done()
	log.Printf("Iniciando consumidor para exchange '%s', fila de consumo '%s', esperando remetente '%s'", exchangeName, queueName, expectedSenderID)

	ch, err := conn.Channel()
	if err != nil {
		log.Printf("Erro ao abrir canal para consumidor da exchange '%s': %v", exchangeName, err)
		return
	}
	defer ch.Close()

	err = ch.ExchangeDeclare(exchangeName, "fanout", true, false, false, false, nil)
	if err != nil {
		log.Printf("Erro ao declarar exchange '%s' para consumidor: %v", exchangeName, err)
		return
	}

	q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
	if err != nil {
		log.Printf("Erro ao declarar fila de consumo '%s': %v", queueName, err)
		return
	}

	err = ch.QueueBind(q.Name, "", exchangeName, false, nil)
	if err != nil {
		log.Printf("Erro ao vincular fila '%s' à exchange '%s': %v", q.Name, exchangeName, err)
		return
	}
	log.Printf("Fila de consumo '%s' vinculada à exchange '%s'", q.Name, exchangeName)

	err = ch.Qos(1, 0, false)
	if err != nil {
		log.Printf("Erro ao configurar QoS para fila '%s': %v", queueName, err)
		return
	}

	consumerTag := fmt.Sprintf("consumer-%s-%s-%d", exchangeName, queueName, time.Now().UnixNano())
	msgs, err := ch.Consume(q.Name, consumerTag, false, false, false, false, nil)
	if err != nil {
		log.Printf("Erro ao registrar consumidor para fila '%s': %v", queueName, err)
		return
	}
	log.Printf("Consumidor '%s' para fila '%s' (exchange '%s') iniciado.", consumerTag, q.Name, exchangeName)

	chErrors := make(chan *amqp.Error)
	ch.NotifyClose(chErrors)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Contexto cancelado para consumidor da fila '%s'. Encerrando...", queueName)
			if errCancel := ch.Cancel(consumerTag, false); errCancel != nil {
				log.Printf("Erro ao cancelar consumidor '%s': %v", consumerTag, errCancel)
			}
			return
		case errClose, ok := <-chErrors:
			if !ok || errClose != nil {
				log.Printf("Canal AMQP para consumidor da fila '%s' fechado (erro: %v, ok: %t).", queueName, errClose, ok)
			}
			return
		case d, ok := <-msgs:
			if !ok {
				log.Printf("Canal de entregas (msgs) para fila '%s' foi fechado.", queueName)
				return
			}
			log.Printf("Recebida mensagem na fila '%s' (DeliveryTag: %d) da exchange '%s'", queueName, d.DeliveryTag, exchangeName)

			// Extrair assinatura e sender_id dos headers
			signature, sigOk := d.Headers["signature"].([]byte)
			senderID, senderOk := d.Headers["sender_id"].(string)

			if !sigOk || !senderOk {
				log.Printf("Header 'signature' ou 'sender_id' ausente ou tipo incorreto para DeliveryTag: %d. Rejeitando.", d.DeliveryTag)
				d.Nack(false, false)
				continue
			}

			if senderID != expectedSenderID {
				log.Printf("SenderID inesperado '%s' (esperado '%s') para DeliveryTag: %d. Rejeitando.", senderID, expectedSenderID, d.DeliveryTag)
				d.Nack(false, false)
				continue
			}

			// Verificar assinatura
			errVerify := verifySignature(d.Body, signature, expectedSenderPublicKey)
			if errVerify != nil {
				log.Printf("Falha na verificação da assinatura para DeliveryTag: %d (remetente: %s): %v. Rejeitando.", d.DeliveryTag, senderID, errVerify)
				d.Nack(false, false) // Não reenfileirar mensagens com assinatura inválida
				continue
			}
			log.Printf("Assinatura verificada com sucesso para DeliveryTag: %d (remetente: %s).", d.DeliveryTag, senderID)

			if errHandler := handlerFunc(d); errHandler != nil {
				log.Printf("Erro no handler para DeliveryTag: %d na fila '%s': %v", d.DeliveryTag, queueName, errHandler)
				d.Nack(false, false)
			} else {
				d.Ack(false)
				log.Printf("Mensagem (DeliveryTag: %d) da fila '%s' processada e confirmada com sucesso.", d.DeliveryTag, queueName)
			}
		}
	}
}

func handlePagamentoAprovado(msg amqp.Delivery) error {
	log.Printf("Handler: Pagamento Aprovado recebido (corpo já verificado): %s", msg.Body)
	var paymentResponse model.PaymentResponse
	if err := json.Unmarshal(msg.Body, &paymentResponse); err != nil {
		return fmt.Errorf("erro ao decodificar JSON de pagamento aprovado: %w", err)
	}
	log.Printf("Pagamento aprovado para ReservationID: %s. Aguardando bilhete.", paymentResponse.ReservationID)
	return nil
}

func handlePagamentoRecusado(msg amqp.Delivery) error {
	log.Printf("Handler: Pagamento Recusado recebido (corpo já verificado): %s", msg.Body)
	var paymentResponse model.PaymentResponse
	if err := json.Unmarshal(msg.Body, &paymentResponse); err != nil {
		return fmt.Errorf("erro ao decodificar JSON de pagamento recusado: %w", err)
	}

	mu.Lock()
	if resultChan, ok := paymentResults[paymentResponse.ReservationID]; ok {
		select {
		case resultChan <- paymentResponse:
			log.Printf("Notificado HTTP handler sobre pagamento recusado para ReservationID: %s", paymentResponse.ReservationID)
		default:
			log.Printf("Não foi possível enviar resultado de pagamento recusado para ReservationID %s: canal de resposta bloqueado ou fechado.", paymentResponse.ReservationID)
		}
	} else {
		log.Printf("Nenhum HTTP handler aguardando por resultado de pagamento recusado para ReservationID: %s", paymentResponse.ReservationID)
	}
	mu.Unlock()
	log.Printf("Cancelando reserva (simulado) para ReservationID: %s devido a pagamento recusado.", paymentResponse.ReservationID)
	return nil
}

func handleBilheteGerado(msg amqp.Delivery) error {
	log.Printf("Handler: Bilhete Gerado recebido (corpo já verificado): %s", msg.Body)
	var bilheteResponse model.BilheteResponse
	if err := json.Unmarshal(msg.Body, &bilheteResponse); err != nil {
		return fmt.Errorf("erro ao decodificar JSON de bilhete gerado: %w", err)
	}

	mu.Lock()
	if respChan, ok := responses[bilheteResponse.ReservationID]; ok {
		select {
		case respChan <- bilheteResponse:
			log.Printf("Enviado bilhete para HTTP handler para ReservationID: %s", bilheteResponse.ReservationID)
		default:
			log.Printf("Não foi possível enviar bilhete para ReservationID %s: canal de resposta bloqueado ou fechado.", bilheteResponse.ReservationID)
		}
	} else {
		log.Printf("Nenhum HTTP handler aguardando por bilhete para ReservationID: %s", bilheteResponse.ReservationID)
	}
	mu.Unlock()
	return nil
}

func listCruisesHandler(c echo.Context) error {
	log.Println("Recebida requisição para listar cruzeiros.")
	if listaDeCruzeiros == nil {
		log.Println("Lista de cruzeiros não carregada.")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Dados de cruzeiros não disponíveis."})
	}
	return c.JSON(http.StatusOK, listaDeCruzeiros)
}

func getCruiseDetailsHandler(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("ID de cruzeiro inválido recebido: %s", idStr)
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ID de cruzeiro inválido"})
	}
	log.Printf("Recebida requisição para detalhes do cruzeiro ID: %d", id)

	for _, cruzeiro := range listaDeCruzeiros {
		if cruzeiro.ID == id {
			return c.JSON(http.StatusOK, cruzeiro)
		}
	}
	log.Printf("Cruzeiro com ID: %d não encontrado.", id)
	return c.JSON(http.StatusNotFound, map[string]string{"error": "Cruzeiro não encontrado"})
}

func createReservationHandler(c echo.Context) error {
	if amqpChannel == nil || amqpConnection == nil || amqpConnection.IsClosed() {
		log.Println("Conexão/Canal AMQP não está pronto para createReservationHandler.")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Serviço de mensageria indisponível"})
	}

	var request model.ReservationRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Dados inválidos para reserva"})
	}
	if request.CruiseID == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "ID do cruzeiro é obrigatório"})
	}
	if request.Customer == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Nome do cliente é obrigatório"})
	}
	if request.Passengers <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Número de passageiros deve ser maior que zero"})
	}

	request.ReservationID = uuid.New().String()
	log.Printf("Criando reserva com ID: %s para cruzeiro ID: %s", request.ReservationID, request.CruiseID)

	bilheteResponseChan := make(chan model.BilheteResponse, 1)
	pagamentoRecusadoChan := make(chan model.PaymentResponse, 1)

	mu.Lock()
	responses[request.ReservationID] = bilheteResponseChan
	paymentResults[request.ReservationID] = pagamentoRecusadoChan
	mu.Unlock()

	defer func() {
		mu.Lock()
		delete(responses, request.ReservationID)
		delete(paymentResults, request.ReservationID)
		mu.Unlock()
		log.Printf("Canais de resposta para ReservationID %s limpos.", request.ReservationID)
	}()

	body, err := json.Marshal(request)
	if err != nil {
		log.Printf("Erro ao codificar JSON da requisição de reserva %s: %v", request.ReservationID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Erro ao processar requisição"})
	}

	// Assinar a mensagem com a chave privada do MSReserva
	signature, err := signMessage(body, msReservaPrivateKey)
	if err != nil {
		log.Printf("Erro ao assinar mensagem de reserva para %s: %v", request.ReservationID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Erro ao assinar mensagem"})
	}

	err = amqpChannel.Publish(
		exchangeReservaCriada,
		"",
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Headers: amqp.Table{
				"signature": signature,
				"sender_id": senderIDMSReserva, // Identificador do remetente
			},
		})
	if err != nil {
		log.Printf("Erro ao publicar mensagem na exchange '%s' para %s: %v", exchangeReservaCriada, request.ReservationID, err)
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Erro ao enviar mensagem de reserva"})
	}
	log.Printf("Mensagem de reserva criada para %s publicada com sucesso na exchange '%s'.", request.ReservationID, exchangeReservaCriada)

	select {
	case bilhete := <-bilheteResponseChan:
		log.Printf("Bilhete recebido para ReservationID %s via HTTP handler.", bilhete.ReservationID)
		return c.JSON(http.StatusOK, bilhete)
	case pagamento := <-pagamentoRecusadoChan:
		log.Printf("Pagamento recusado para ReservationID %s via HTTP handler.", pagamento.ReservationID)
		return c.JSON(http.StatusAccepted, map[string]string{
			"status":  pagamento.Status,
			"message": pagamento.Message,
			"details": fmt.Sprintf("Pagamento para reserva %s foi recusado.", pagamento.ReservationID),
		})
	case <-time.After(30 * time.Second):
		log.Printf("Timeout ao aguardar resposta para ReservationID %s.", request.ReservationID)
		return c.JSON(http.StatusRequestTimeout, map[string]string{"error": "Timeout ao processar a reserva"})
	}
}

func main() {
	log.Println("Iniciando microserviço de reservas...")
	loadCertificates()

	cruzeirosFilePath := os.Getenv("CRUZEIROS_JSON_PATH")
	if cruzeirosFilePath == "" {
		cruzeirosFilePath = "data/cruzeiros.json"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			cruzeirosFilePath = "/app/cruzeiros.json"
		}
	}
	errLoad := loadCruzeirosData(cruzeirosFilePath)
	failOnError(errLoad, fmt.Sprintf("Falha ao carregar dados dos cruzeiros de %s", cruzeirosFilePath))

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	wg.Add(1)
	go setupRabbitMQ(ctx, &wg)

	e := echo.New()
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, echo.HeaderAuthorization},
	}))

	e.GET("/cruzeiros", listCruisesHandler)
	e.GET("/cruzeiros/:id", getCruiseDetailsHandler)
	e.POST("/reservations", createReservationHandler)

	go func() {
		httpPort := os.Getenv("HTTP_PORT_MSRESERVA")
		if httpPort == "" {
			httpPort = "8080"
		}
		log.Printf("Servidor de reservas HTTP escutando na porta %s", httpPort)
		if err := e.Start(":" + httpPort); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Erro ao iniciar servidor HTTP: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Recebido sinal de encerramento, iniciando shutdown gracioso...")

	cancel()

	shutdownCtxHttp, shutdownCancelHttp := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancelHttp()
	if err := e.Shutdown(shutdownCtxHttp); err != nil {
		log.Printf("Erro no shutdown do servidor HTTP: %v", err)
	} else {
		log.Println("Servidor HTTP encerrado.")
	}

	log.Println("Aguardando todas as goroutines RabbitMQ terminarem...")
	wgDone := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgDone)
	}()

	select {
	case <-wgDone:
		log.Println("Todas as goroutines RabbitMQ terminaram.")
	case <-time.After(15 * time.Second):
		log.Println("Timeout esperando goroutines RabbitMQ.")
	}
	log.Println("Microserviço de reservas encerrado.")
}
