package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"github.com/google/uuid"
	"github.com/streadway/amqp"
	"io/ioutil"
	"log"
	mathRand "math/rand"
	"os"
	"reserva-cruzeiros/model"
	"time"
)

var (
	// Chave privada deste microsserviço (MSPagamento)
	msPagamentoPrivateKey *rsa.PrivateKey
	// Chave pública do MSReserva (para verificar mensagens de reserva_criada)
	msReservaPublicKey *rsa.PublicKey
	// Chaves públicas dos destinatários das mensagens que MSPagamento envia
	// (não são usadas para verificar, mas para referência de quem são os destinatários)
	// msBilhetePublicKey *rsa.PublicKey - Não é mais necessário carregar aqui para enviar no header
)

const (
	exchangeReservaCriada     = "reserva_criada"
	exchangePagamentoAprovado = "pagamento_aprovado"
	exchangePagamentoRecusado = "pagamento_recusado"
)

const (
	filaConsumoReservasCriadas = "mspagamento_consumo_reserva_criada"
)

const (
	senderIDMSReserva   = "msreserva"
	senderIDMSPagamento = "mspagamento"
	// senderIDMSBilhete   = "msbilhete" // Não é usado diretamente por mspagamento para enviar
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
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
	// Carregar chave privada do MSPagamento
	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH_MSPAGAMENTO")
	if privateKeyPath == "" {
		privateKeyPath = "certificates/mspagamento/private_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			privateKeyPath = "/app/certs/mspagamento/private_key.pem"
		}
	}
	msPagamentoPrivateKey, err = loadPrivateKey(privateKeyPath)
	failOnError(err, fmt.Sprintf("MSPagamento: Não foi possível carregar a chave privada de %s", privateKeyPath))
	log.Println("MSPagamento: Chave privada carregada com sucesso.")

	// Carregar chave pública do MSReserva (para verificar mensagens de reserva_criada)
	msReservaPublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSRESERVA")
	if msReservaPublicKeyPath == "" {
		msReservaPublicKeyPath = "certificates/msreserva/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msReservaPublicKeyPath = "/app/certs/msreserva/public_key.pem"
		}
	}
	msReservaPublicKey, err = loadPublicKey(msReservaPublicKeyPath)
	failOnError(err, fmt.Sprintf("MSPagamento: Não foi possível carregar a chave pública do MSReserva de %s", msReservaPublicKeyPath))
	log.Println("MSPagamento: Chave pública do MSReserva carregada com sucesso.")
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

func processPayment(request model.ReservationRequest) (model.PaymentResponse, bool) {
	paymentID := uuid.New().String()
	valid := mathRand.Intn(5) != 0
	if !valid {
		log.Printf("Pagamento recusado para ReservationID: %s", request.ReservationID)
		return model.PaymentResponse{
			PaymentID:     paymentID,
			ReservationID: request.ReservationID,
			CruiseID:      request.CruiseID,
			Customer:      request.Customer,
			Status:        "failed",
			Message:       "Pagamento Recusado",
			IssueDate:     time.Now(),
		}, false
	}
	log.Printf("Pagamento aprovado para ReservationID: %s", request.ReservationID)
	time.Sleep(500 * time.Millisecond)
	return model.PaymentResponse{
		PaymentID:     paymentID,
		ReservationID: request.ReservationID,
		CruiseID:      request.CruiseID,
		Customer:      request.Customer,
		Status:        "approved",
		Message:       "Pagamento aprovado com sucesso",
		IssueDate:     time.Now(),
	}, true
}

func main() {
	log.Println("Iniciando microserviço de pagamento...")
	loadCertificates()
	mathRand.Seed(time.Now().UnixNano())
	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}

	var conn *amqp.Connection
	var err error
	maxRetries := 5
	retryDelay := 5 * time.Second

	for i := 0; i < maxRetries; i++ {
		conn, err = amqp.Dial(rabbitMQURL)
		if err == nil {
			log.Println("Conectado ao RabbitMQ com sucesso.")
			break
		}
		log.Printf("Falha ao conectar ao RabbitMQ (tentativa %d/%d): %v. Tentando novamente em %v...", i+1, maxRetries, err, retryDelay)
		time.Sleep(retryDelay)
	}
	failOnError(err, "Falha ao conectar ao RabbitMQ após múltiplas tentativas")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Falha ao abrir canal RabbitMQ")
	defer ch.Close()

	err = ch.ExchangeDeclare(exchangeReservaCriada,
		"fanout",
		true,
		false,
		false,
		false,
		nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange '%s'", exchangeReservaCriada))

	qReservas, err := ch.QueueDeclare(filaConsumoReservasCriadas,
		true,
		false,
		false,
		false,
		nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar fila de consumo '%s'", filaConsumoReservasCriadas))

	err = ch.QueueBind(qReservas.Name,
		"",
		exchangeReservaCriada,
		false,
		nil)
	failOnError(err, fmt.Sprintf("Falha ao vincular fila '%s' à exchange '%s'", qReservas.Name, exchangeReservaCriada))
	log.Printf("Fila de consumo '%s' vinculada à exchange '%s'", qReservas.Name, exchangeReservaCriada)

	err = ch.ExchangeDeclare(exchangePagamentoAprovado,
		"fanout",
		true,
		false,
		false,
		false,
		nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange de publicação '%s'", exchangePagamentoAprovado))
	err = ch.ExchangeDeclare(exchangePagamentoRecusado,
		"fanout",
		true,
		false,
		false,
		false,
		nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange de publicação '%s'", exchangePagamentoRecusado))
	log.Printf("Exchanges de publicação '%s' e '%s' (fanout) declaradas.", exchangePagamentoAprovado, exchangePagamentoRecusado)

	err = ch.Qos(1, 0, false)
	failOnError(err, "Falha ao configurar QoS")

	msgsReservasCriadas, err := ch.Consume(
		qReservas.Name, "", false, false, false, false, nil,
	)
	failOnError(err, "Falha ao registrar consumidor para 'reserva_criada'")
	log.Printf("Microserviço de pagamento aguardando mensagens da exchange '%s' na fila '%s'.", exchangeReservaCriada, qReservas.Name)

	forever := make(chan bool)

	go func() {
		for msg := range msgsReservasCriadas {
			log.Printf("Recebida mensagem de reserva criada (DeliveryTag: %d): %s", msg.DeliveryTag, msg.Body)

			signature, sigOk := msg.Headers["signature"].([]byte)
			senderID, senderOk := msg.Headers["sender_id"].(string)

			if !sigOk || !senderOk {
				log.Printf("Header 'signature' ou 'sender_id' ausente ou tipo incorreto para DeliveryTag: %d. Rejeitando.", msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}

			if senderID != senderIDMSReserva {
				log.Printf("SenderID inesperado '%s' (esperado '%s') para DeliveryTag: %d. Rejeitando.", senderID, senderIDMSReserva, msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}

			errVerify := verifySignature(msg.Body, signature, msReservaPublicKey)
			if errVerify != nil {
				log.Printf("Falha na verificação da assinatura para DeliveryTag: %d (remetente: %s): %v. Rejeitando.", msg.DeliveryTag, senderID, errVerify)
				msg.Nack(false, false)
				continue
			}
			log.Printf("Assinatura da mensagem de reserva_criada verificada com sucesso para DeliveryTag: %d.", msg.DeliveryTag)

			var reservaRequest model.ReservationRequest
			if err := json.Unmarshal(msg.Body, &reservaRequest); err != nil {
				log.Printf("Erro ao decodificar JSON da mensagem de reserva (DeliveryTag: %d): %s. Rejeitando.", msg.DeliveryTag, err)
				msg.Nack(false, false)
				continue
			}

			paymentResponse, aprovado := processPayment(reservaRequest)
			bodyResp, errMarshal := json.Marshal(paymentResponse)
			if errMarshal != nil {
				log.Printf("Erro ao codificar resposta JSON (DeliveryTag: %d): %s. Rejeitando.", msg.DeliveryTag, errMarshal)
				msg.Nack(false, false)
				continue
			}

			// Assinar a resposta com a chave privada do MSPagamento
			signatureResp, errSign := signMessage(bodyResp, msPagamentoPrivateKey)
			if errSign != nil {
				log.Printf("Erro ao assinar resposta de pagamento para DeliveryTag: %d: %v. Rejeitando.", msg.DeliveryTag, errSign)
				msg.Nack(false, false) // Não podemos prosseguir sem assinar
				continue
			}

			var targetExchange string
			if aprovado {
				targetExchange = exchangePagamentoAprovado
			} else {
				targetExchange = exchangePagamentoRecusado
			}

			// Publicar para a exchange apropriada (pagamento_aprovado ou pagamento_recusado)
			// Ambas são consumidas pelo MSReserva. A de pagamento_aprovado também pelo MSBilhete.
			// O sender_id é sempre mspagamento.
			errPub := ch.Publish(
				targetExchange,
				"", // Routing key é ignorada para fanout
				false,
				false,
				amqp.Publishing{
					DeliveryMode: amqp.Persistent,
					ContentType:  "application/json",
					Body:         bodyResp,
					Headers: amqp.Table{
						"signature": signatureResp,
						"sender_id": senderIDMSPagamento,
					},
				})

			if errPub != nil {
				log.Printf("Erro ao publicar para '%s' (DeliveryTag: %d): %s. Nack com requeue.", targetExchange, msg.DeliveryTag, errPub)
				msg.Nack(false, true) // Tentar reprocessar
				continue
			}
			log.Printf("Publicado na exchange '%s' (ReservationID: %s, DeliveryTag: %d).", targetExchange, paymentResponse.ReservationID, msg.DeliveryTag)

			log.Printf("Confirmando mensagem original (DeliveryTag: %d) após publicações.", msg.DeliveryTag)
			msg.Ack(false)
		}
		log.Println("Canal de mensagens 'msgsReservasCriadas' foi fechado. Encerrando goroutine de consumo.")
	}()

	<-forever
	log.Println("Microserviço de pagamento encerrando.")
}
