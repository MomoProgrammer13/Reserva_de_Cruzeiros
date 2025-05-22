package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reserva-cruzeiros/model"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

var (
	privateKey         *rsa.PrivateKey
	msreservaPublicKey *rsa.PublicKey
)

const (
	exchangePagamentoAprovado = "pagamento_aprovado"
	exchangeBilheteGerado     = "bilhete_gerado"
)

const (
	filaConsumoPagamentoAprovadoMsBilhete = "msbilhete_consumo_pagamento_aprovado"
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
	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH_MSBILHETE")
	if privateKeyPath == "" {
		privateKeyPath = "certificates/msbilhete/private_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			privateKeyPath = "/app/certs/msbilhete/private_key.pem"
		}
	}
	privateKey, err = loadPrivateKey(privateKeyPath)
	failOnError(err, fmt.Sprintf("Não foi possível carregar a chave privada do msbilhete de %s", privateKeyPath))
	log.Println("Chave privada do msbilhete carregada com sucesso.")

	msreservaPublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSRESERVA")
	if msreservaPublicKeyPath == "" {
		msreservaPublicKeyPath = "certificates/msreserva/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msreservaPublicKeyPath = "/app/certs/msreserva/public_key.pem"
		}
	}
	msreservaPublicKey, err = loadPublicKey(msreservaPublicKeyPath)
	failOnError(err, fmt.Sprintf("Não foi possível carregar a chave pública do msreserva de %s", msreservaPublicKeyPath))
	log.Println("Chave pública do msreserva (destinatário) carregada com sucesso.")
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

func processBilhete(request model.PaymentResponse) model.BilheteResponse {
	log.Printf("Processando bilhete para ReservationID: %s", request.ReservationID)
	time.Sleep(500 * time.Millisecond)
	return model.BilheteResponse{
		TicketID:      uuid.New().String(),
		ReservationID: request.ReservationID,
		Customer:      request.Customer,
		CruiseID:      request.CruiseID,
		Status:        "Emitido",
		Message:       "Bilhete emitido com sucesso",
		IssuedAt:      time.Now(),
	}
}

func main() {
	log.Println("Iniciando microserviço de bilhetes...")
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

	// Declarar a exchange da qual MSBilhete consome (idempotente)
	err = ch.ExchangeDeclare(exchangePagamentoAprovado, "fanout", true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange '%s'", exchangePagamentoAprovado))

	// Declarar a fila de consumo para MSBilhete
	qPagamento, err := ch.QueueDeclare(filaConsumoPagamentoAprovadoMsBilhete, true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar fila de consumo '%s'", filaConsumoPagamentoAprovadoMsBilhete))

	// Vincular fila de consumo à exchange
	err = ch.QueueBind(qPagamento.Name, "", exchangePagamentoAprovado, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao vincular fila '%s' à exchange '%s'", qPagamento.Name, exchangePagamentoAprovado))
	log.Printf("Fila de consumo '%s' vinculada à exchange '%s'", qPagamento.Name, exchangePagamentoAprovado)

	// Declarar exchange para onde MSBilhete publica (idempotente)
	err = ch.ExchangeDeclare(exchangeBilheteGerado, "fanout", true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange de publicação '%s'", exchangeBilheteGerado))
	log.Printf("Exchange de publicação '%s' (fanout) declarada.", exchangeBilheteGerado)

	err = ch.Qos(1, 0, false)
	failOnError(err, "Falha ao configurar QoS")

	msgsPagamentoAprovado, err := ch.Consume(
		qPagamento.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao registrar consumidor para 'pagamento_aprovado'")
	log.Printf("Microserviço de bilhetes aguardando mensagens da exchange '%s' na fila '%s'.", exchangePagamentoAprovado, qPagamento.Name)

	forever := make(chan bool)

	go func() {
		for msg := range msgsPagamentoAprovado {
			log.Printf("Recebido pagamento aprovado (DeliveryTag: %d): %s", msg.DeliveryTag, msg.Body)

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
			// Valida se a mensagem foi endereçada ao MSBilhete (chave pública do MSBilhete no header)
			if !validateKeyPair(receivedPublicKey, privateKey) {
				log.Printf("Chave pública recebida não corresponde à chave privada do msbilhete para DeliveryTag: %d. Rejeitando.", msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}
			log.Printf("Chave pública (do destinatário msbilhete) validada com sucesso para DeliveryTag: %d.", msg.DeliveryTag)

			var paymentResponse model.PaymentResponse
			if err := json.Unmarshal(msg.Body, &paymentResponse); err != nil {
				log.Printf("Erro ao decodificar JSON da mensagem de pagamento (DeliveryTag: %d): %s. Rejeitando.", msg.DeliveryTag, err)
				msg.Nack(false, false)
				continue
			}

			bilheteResponse := processBilhete(paymentResponse)
			bodyResp, errMarshal := json.Marshal(bilheteResponse)
			if errMarshal != nil {
				log.Printf("Erro ao codificar resposta JSON do bilhete (DeliveryTag: %d): %s. Rejeitando.", msg.DeliveryTag, errMarshal)
				msg.Nack(false, false)
				continue
			}

			// MSBilhete publica para MSReserva. Header 'key' deve conter a chave pública do MSReserva.
			msreservaKeyPem, errKeyPem := publicKeyToPEM(msreservaPublicKey)
			if errKeyPem != nil {
				log.Printf("Erro ao converter chave pública do msreserva para PEM (DeliveryTag: %d): %v. Rejeitando.", msg.DeliveryTag, errKeyPem)
				msg.Nack(false, false)
				continue
			}

			errPublish := ch.Publish(
				exchangeBilheteGerado,
				"",
				false,
				false,
				amqp.Publishing{
					DeliveryMode: amqp.Persistent,
					ContentType:  "application/json",
					Body:         bodyResp,
					Headers:      amqp.Table{"key": msreservaKeyPem},
				})

			if errPublish != nil {
				log.Printf("Erro ao publicar na exchange '%s' (DeliveryTag: %d): %s. Nack com requeue.", exchangeBilheteGerado, msg.DeliveryTag, errPublish)
				msg.Nack(false, true)
			} else {
				log.Printf("Publicado na exchange '%s' (ReservationID: %s, DeliveryTag: %d). Confirmando mensagem original.", exchangeBilheteGerado, bilheteResponse.ReservationID, msg.DeliveryTag)
				msg.Ack(false)
			}
		}
		log.Println("Canal de mensagens 'msgsPagamentoAprovado' foi fechado. Encerrando goroutine de consumo.")
	}()

	<-forever
	log.Println("Microserviço de bilhetes encerrando.")
}
