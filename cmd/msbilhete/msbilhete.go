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
	"io/ioutil"
	"log"
	"os"
	"reserva-cruzeiros/model"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

var (
	// Chave privada deste microsserviço (MSBilhete)
	msBilhetePrivateKey *rsa.PrivateKey
	// Chave pública do MSPagamento (para verificar mensagens de pagamento_aprovado)
	msPagamentoPublicKey *rsa.PublicKey
	// Chave pública do MSReserva (para referência, já que MSBilhete envia msg para MSReserva)
	// Não é usada para verificar mensagens recebidas por MSBilhete, mas para construir o header da msg enviada.
	// No novo modelo de assinatura, MSBilhete assina com sua chave privada.
	// O header "sender_id" identificará MSBilhete. MSReserva usará a msBilhetePublicKey que já possui.
	// msReservaPublicKey *rsa.PublicKey
)

const (
	exchangePagamentoAprovado = "pagamento_aprovado"
	exchangeBilheteGerado     = "bilhete_gerado"
)

const (
	filaConsumoPagamentoAprovadoMsBilhete = "msbilhete_consumo_pagamento_aprovado"
)

const (
	// senderIDMSReserva   = "msreserva" // Não usado por msbilhete
	senderIDMSPagamento = "mspagamento"
	senderIDMSBilhete   = "msbilhete"
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
	// Carregar chave privada do MSBilhete
	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH_MSBILHETE")
	if privateKeyPath == "" {
		privateKeyPath = "certificates/msbilhete/private_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			privateKeyPath = "/app/certs/msbilhete/private_key.pem"
		}
	}
	msBilhetePrivateKey, err = loadPrivateKey(privateKeyPath)
	failOnError(err, fmt.Sprintf("MSBilhete: Não foi possível carregar a chave privada de %s", privateKeyPath))
	log.Println("MSBilhete: Chave privada carregada com sucesso.")

	// Carregar chave pública do MSPagamento (para verificar mensagens de pagamento_aprovado)
	msPagamentoPublicKeyPath := os.Getenv("PUBLIC_KEY_PATH_MSPAGAMENTO")
	if msPagamentoPublicKeyPath == "" {
		msPagamentoPublicKeyPath = "certificates/mspagamento/public_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			msPagamentoPublicKeyPath = "/app/certs/mspagamento/public_key.pem"
		}
	}
	msPagamentoPublicKey, err = loadPublicKey(msPagamentoPublicKeyPath)
	failOnError(err, fmt.Sprintf("MSBilhete: Não foi possível carregar a chave pública do MSPagamento de %s", msPagamentoPublicKeyPath))
	log.Println("MSBilhete: Chave pública do MSPagamento carregada com sucesso.")
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

	err = ch.ExchangeDeclare(exchangePagamentoAprovado,
		"fanout",
		true, false,
		false, false,
		nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange '%s'", exchangePagamentoAprovado))

	qPagamento, err := ch.QueueDeclare(filaConsumoPagamentoAprovadoMsBilhete, true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar fila de consumo '%s'", filaConsumoPagamentoAprovadoMsBilhete))

	err = ch.QueueBind(qPagamento.Name, "", exchangePagamentoAprovado, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao vincular fila '%s' à exchange '%s'", qPagamento.Name, exchangePagamentoAprovado))
	log.Printf("Fila de consumo '%s' vinculada à exchange '%s'", qPagamento.Name, exchangePagamentoAprovado)

	err = ch.ExchangeDeclare(exchangeBilheteGerado, "fanout", true, false, false, false, nil)
	failOnError(err, fmt.Sprintf("Falha ao declarar exchange de publicação '%s'", exchangeBilheteGerado))
	log.Printf("Exchange de publicação '%s' (fanout) declarada.", exchangeBilheteGerado)

	err = ch.Qos(1, 0, false)
	failOnError(err, "Falha ao configurar QoS")

	msgsPagamentoAprovado, err := ch.Consume(
		qPagamento.Name, "", false, false, false, false, nil,
	)
	failOnError(err, "Falha ao registrar consumidor para 'pagamento_aprovado'")
	log.Printf("Microserviço de bilhetes aguardando mensagens da exchange '%s' na fila '%s'.", exchangePagamentoAprovado, qPagamento.Name)

	forever := make(chan bool)

	go func() {
		for msg := range msgsPagamentoAprovado {
			log.Printf("Recebido pagamento aprovado (DeliveryTag: %d): %s", msg.DeliveryTag, msg.Body)

			signature, sigOk := msg.Headers["signature"].([]byte)
			senderID, senderOk := msg.Headers["sender_id"].(string)

			if !sigOk || !senderOk {
				log.Printf("Header 'signature' ou 'sender_id' ausente ou tipo incorreto para DeliveryTag: %d. Rejeitando.", msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}

			if senderID != senderIDMSPagamento {
				log.Printf("SenderID inesperado '%s' (esperado '%s') para DeliveryTag: %d. Rejeitando.", senderID, senderIDMSPagamento, msg.DeliveryTag)
				msg.Nack(false, false)
				continue
			}

			errVerify := verifySignature(msg.Body, signature, msPagamentoPublicKey)
			if errVerify != nil {
				log.Printf("Falha na verificação da assinatura para DeliveryTag: %d (remetente: %s): %v. Rejeitando.", msg.DeliveryTag, senderID, errVerify)
				msg.Nack(false, false)
				continue
			}
			log.Printf("Assinatura da mensagem de pagamento_aprovado verificada com sucesso para DeliveryTag: %d.", msg.DeliveryTag)

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

			// Assinar a resposta com a chave privada do MSBilhete
			signatureResp, errSign := signMessage(bodyResp, msBilhetePrivateKey)
			if errSign != nil {
				log.Printf("Erro ao assinar resposta de bilhete para DeliveryTag: %d: %v. Rejeitando.", msg.DeliveryTag, errSign)
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
					Headers: amqp.Table{
						"signature": signatureResp,
						"sender_id": senderIDMSBilhete,
					},
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
