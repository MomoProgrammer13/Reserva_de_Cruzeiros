package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
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

var (
	privateKey         *rsa.PrivateKey
	msbilhetePublicKey *rsa.PublicKey // Chave pública do MSBilhete (destinatário)
	msreservaPublicKey *rsa.PublicKey // Chave pública do MSReserva (destinatário)
)

const (
	exchangeReservaCriada     = "reserva_criada"     // MSPagamento consome desta
	exchangePagamentoAprovado = "pagamento_aprovado" // MSPagamento publica nesta
	exchangePagamentoRecusado = "pagamento_recusado" // MSPagamento publica nesta
)

// Nome da fila que o MSPagamento usará para consumir da exchange "reserva_criada"
const (
	filaConsumoReservasCriadas = "mspagamento_consumo_reserva_criada"
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
	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH_MSPAGAMENTO")
	if privateKeyPath == "" {
		privateKeyPath = "certificates/mspagamento/private_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			privateKeyPath = "/app/certs/mspagamento/private_key.pem"
		}
	}
	privateKey, err = loadPrivateKey(privateKeyPath)
	failOnError(err, fmt.Sprintf("Não foi possível carregar a chave privada do mspagamento de %s", privateKeyPath))
	log.Println("Chave privada do mspagamento carregada com sucesso.")

	msbilhetePublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSBILHETE")
	if msbilhetePublicKeyPath == "" {
		msbilhetePublicKeyPath = "certificates/msbilhete/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msbilhetePublicKeyPath = "/app/certs/msbilhete/public_key.pem"
		}
	}
	msbilhetePublicKey, err = loadPublicKey(msbilhetePublicKeyPath)
	failOnError(err, fmt.Sprintf("Não foi possível carregar a chave pública do msbilhete de %s", msbilhetePublicKeyPath))
	log.Println("Chave pública do msbilhete carregada com sucesso.")

	msreservaPublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSRESERVA")
	if msreservaPublicKeyPath == "" {
		msreservaPublicKeyPath = "certificates/msreserva/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msreservaPublicKeyPath = "/app/certs/msreserva/public_key.pem"
		}
	}
	msreservaPublicKey, err = loadPublicKey(msreservaPublicKeyPath)
	failOnError(err, fmt.Sprintf("Não foi possível carregar a chave pública do msreserva de %s", msreservaPublicKeyPath))
	log.Println("Chave pública do msreserva carregada com sucesso.")
}

func validateKeyPair(publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) bool {
	return publicKey.N.Cmp(privateKey.N) == 0 && publicKey.E == privateKey.E
}

func publicKeyToPEM(publicKey *rsa.PublicKey) (string, error) {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", fmt.Errorf("falha ao serializar chave pública para PKIX: %w", err)
	}
	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	})
	return string(publicKeyPEM), nil
}

func pemToPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave pública")
	}
	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("falha ao parsear chave pública PKIX do PEM: %w", err)
	}
	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("chave PEM não é do tipo RSA Public Key")
	}
	return publicKey, nil
}

func processPayment(request model.ReservationRequest) (model.PaymentResponse, bool) {
	paymentID := uuid.New().String()
	valid := rand.Intn(100) < 80 // Simulação, aprova todos
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
	rand.Seed(time.Now().UnixNano())
	loadCertificates()

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

	// Declarar a exchange da qual MSPagamento consome (idempotente)
	err = ch.ExchangeDeclare(exchangeReservaCriada, "fanout", true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange '%s'", exchangeReservaCriada))

	// Declarar a fila de consumo para MSPagamento
	qReservas, err := ch.QueueDeclare(filaConsumoReservasCriadas, true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar fila de consumo '%s'", filaConsumoReservasCriadas))

	// Vincular fila de consumo à exchange
	err = ch.QueueBind(qReservas.Name, "", exchangeReservaCriada, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao vincular fila '%s' à exchange '%s'", qReservas.Name, exchangeReservaCriada))
	log.Printf("Fila de consumo '%s' vinculada à exchange '%s'", qReservas.Name, exchangeReservaCriada)

	// Declarar exchanges para onde MSPagamento publica (idempotente)
	err = ch.ExchangeDeclare(exchangePagamentoAprovado, "fanout", true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange de publicação '%s'", exchangePagamentoAprovado))
	err = ch.ExchangeDeclare(exchangePagamentoRecusado, "fanout", true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange de publicação '%s'", exchangePagamentoRecusado))
	log.Printf("Exchanges de publicação '%s' e '%s' (fanout) declaradas.", exchangePagamentoAprovado, exchangePagamentoRecusado)

	err = ch.Qos(1, 0, false)
	failOnError(err, "Falha ao configurar QoS")

	msgsReservasCriadas, err := ch.Consume(
		qReservas.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao registrar consumidor para 'reserva_criada'")
	log.Printf("Microserviço de pagamento aguardando mensagens da exchange '%s' na fila '%s'.", exchangeReservaCriada, qReservas.Name)

	forever := make(chan bool)

	go func() {
		for msg := range msgsReservasCriadas {
			log.Printf("Recebida mensagem de reserva criada (DeliveryTag: %d): %s", msg.DeliveryTag, msg.Body)

			var receivedPublicKeyPEM string
			var keyOk bool
			if headerKey, headerExists := msg.Headers["key"]; headerExists {
				receivedPublicKeyPEM, keyOk = headerKey.(string)
			}
			if !keyOk {
				log.Printf("Chave pública não encontrada ou tipo inválido nos headers para DeliveryTag: %d. Rejeitando.", msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}
			receivedPublicKey, errKey := pemToPublicKey(receivedPublicKeyPEM)
			if errKey != nil {
				log.Printf("Erro ao converter PEM para chave pública para DeliveryTag: %d: %v. Rejeitando.", msg.DeliveryTag, errKey)
				msg.Nack(false, false)
				continue
			}
			// Valida se a mensagem foi endereçada ao MSPagamento (chave pública do MSPagamento no header)
			if !validateKeyPair(receivedPublicKey, privateKey) {
				log.Printf("Chave pública recebida não corresponde à chave privada do mspagamento para DeliveryTag: %d. Rejeitando.", msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}
			log.Printf("Chave pública (do destinatário mspagamento) validada com sucesso para DeliveryTag: %d.", msg.DeliveryTag)

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

			var targetExchange string
			var keyPEMDestinatario string

			if aprovado {
				targetExchange = exchangePagamentoAprovado

				keyPEMReserva, errKey := publicKeyToPEM(msreservaPublicKey)
				if errKey != nil {
					log.Printf("Erro ao obter PEM da chave pública do MSReserva: %v. Nack msg %d", errKey, msg.DeliveryTag)
					msg.Nack(false, false)
					continue
				}
				// Para MSBilhete:
				keyPEMBilhete, errKey := publicKeyToPEM(msbilhetePublicKey)
				if errKey != nil {
					log.Printf("Erro ao obter PEM da chave pública do MSBilhete: %v. Nack msg %d", errKey, msg.DeliveryTag)
					msg.Nack(false, false)
					continue
				}

				errPubRes := ch.Publish(targetExchange, "", false, false, amqp.Publishing{
					DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: bodyResp,
					Headers: amqp.Table{"key": keyPEMReserva},
				})
				if errPubRes != nil {
					log.Printf("Erro ao publicar para MSReserva na exchange '%s' (DeliveryTag: %d): %s. Nack com requeue.", targetExchange, msg.DeliveryTag, errPubRes)
					msg.Nack(false, true)
					continue
				}
				log.Printf("Publicado na exchange '%s' para MSReserva (ReservationID: %s).", targetExchange, paymentResponse.ReservationID)

				errPubBil := ch.Publish(targetExchange, "", false, false, amqp.Publishing{
					DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: bodyResp,
					Headers: amqp.Table{"key": keyPEMBilhete},
				})
				if errPubBil != nil {
					log.Printf("Erro ao publicar para MSBilhete na exchange '%s' (DeliveryTag: %d): %s. Nack com requeue.", targetExchange, msg.DeliveryTag, errPubBil)

					msg.Nack(false, true)
					continue
				}
				log.Printf("Publicado na exchange '%s' para MSBilhete (ReservationID: %s).", targetExchange, paymentResponse.ReservationID)

			} else {
				targetExchange = exchangePagamentoRecusado
				keyPEMDestinatario, err = publicKeyToPEM(msreservaPublicKey)
				if err != nil {
					log.Printf("Erro ao obter PEM da chave pública do MSReserva (recusado): %v. Nack msg %d", err, msg.DeliveryTag)
					msg.Nack(false, false)
					continue
				}
				errPub := ch.Publish(targetExchange, "", false, false, amqp.Publishing{
					DeliveryMode: amqp.Persistent, ContentType: "application/json", Body: bodyResp,
					Headers: amqp.Table{"key": keyPEMDestinatario},
				})
				if errPub != nil {
					log.Printf("Erro ao publicar na exchange '%s' (DeliveryTag: %d): %s. Nack com requeue.", targetExchange, msg.DeliveryTag, errPub)
					msg.Nack(false, true)
					continue
				}
				log.Printf("Publicado na exchange '%s' para MSReserva (ReservationID: %s).", targetExchange, paymentResponse.ReservationID)
			}

			log.Printf("Confirmando mensagem original (DeliveryTag: %d) após publicações.", msg.DeliveryTag)
			msg.Ack(false)
		}
		log.Println("Canal de mensagens 'msgsReservasCriadas' foi fechado. Encerrando goroutine de consumo.")
	}()

	<-forever
	log.Println("Microserviço de pagamento encerrando.")
}
