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

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	privateKeyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(privateKeyData)
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave privada")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	return privateKey, nil
}

func loadPublicKey(path string) (*rsa.PublicKey, error) {
	publicKeyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	block, _ := pem.Decode(publicKeyData)
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave pública")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("chave não é do tipo RSA Public Key")
	}

	return publicKey, nil
}

func loadCertificates() {
	var err error

	// Carregar chave privada do microserviço msbilhete
	privateKeyPath := "/app/certs/msbilhete/private_key.pem"
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		privateKeyPath = "certificates/msbilhete/private_key.pem"
	}

	privateKey, err = loadPrivateKey(privateKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave privada: %v", err)
	} else {
		log.Println("Chave privada carregada com sucesso")
	}

	// Carregar chave pública do microserviço msreserva
	msreservaPublicKeyPath := "/app/certs/msreserva/public_key.pem"
	if _, err := os.Stat(msreservaPublicKeyPath); os.IsNotExist(err) {
		msreservaPublicKeyPath = "certificates/msreserva/public_key.pem"
	}

	msreservaPublicKey, err = loadPublicKey(msreservaPublicKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave pública do msreserva: %v", err)
	} else {
		log.Println("Chave pública do msreserva carregada com sucesso")
	}
}

// Função para verificar se uma chave pública corresponde a uma chave privada
func validateKeyPair(publicKey *rsa.PublicKey, privateKey *rsa.PrivateKey) bool {
	// Comparação direta dos componentes da chave
	return publicKey.N.Cmp(privateKey.N) == 0 && publicKey.E == privateKey.E
}

// Função para converter chave pública para formato PEM
func publicKeyToPEM(publicKey *rsa.PublicKey) (string, error) {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return "", err
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM), nil
}

// Função para converter string PEM para chave pública
func pemToPublicKey(pemStr string) (*rsa.PublicKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave pública")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("chave não é do tipo RSA Public Key")
	}

	return publicKey, nil
}

// Simulação de processamento de reserva
func processBilhete(request model.PaymentResponse) model.BilheteResponse {

	// Simular processamento
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

	loadCertificates()

	// Tenta conectar ao RabbitMQ com retry
	var conn *amqp.Connection
	var err error

	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial("amqp://guest:guest@localhost:5672/")
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

	err = ch.ExchangeDeclare(
		"bilhete_gerado",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de bilhetes")

	pa, err := ch.QueueDeclare(
		"pagamento_aprovado",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de pagamentos aprovados")

	msgsPagamentoAprovado, err := ch.Consume(
		pa.Name, // queue
		"",      // consumer
		false,   // auto-ack (mudado para false para confirmar manualmente)
		false,   // exclusive
		false,   // no-local
		false,   // no-wait
		nil,     // args
	)
	failOnError(err, "Falha ao registrar consumidor")

	forever := make(chan bool)

	go func() {
		for msg := range msgsPagamentoAprovado {
			log.Printf("Recebido pagamento aprovado: %s", msg.Body)

			// Verificar se há uma chave pública nos headers
			var receivedPublicKeyPEM string
			var ok bool

			if receivedPublicKeyPEM, ok = msg.Headers["key"].(string); !ok {
				log.Printf("Chave pública não encontrada nos headers, rejeitando mensagem")
				msg.Nack(false, false) // Rejeitar mensagem sem requeue
				continue
			}

			// Converter PEM para chave pública
			receivedPublicKey, err := pemToPublicKey(receivedPublicKeyPEM)
			if err != nil {
				log.Printf("Erro ao converter PEM para chave pública: %v", err)
				msg.Nack(false, false)
				continue
			}

			// Validar se a chave pública recebida corresponde à chave privada do msbilhete
			if !validateKeyPair(receivedPublicKey, privateKey) {
				log.Printf("A chave pública recebida não corresponde à chave privada do msbilhete")
				msg.Nack(false, true)
				continue
			}

			log.Printf("Chave pública validada com sucesso, processando pagamento aprovado")

			var paymentResponse model.PaymentResponse
			if err := json.Unmarshal(msg.Body, &paymentResponse); err != nil {
				log.Printf("Erro ao decodificar mensagem de pagamento: %s", err)
				msg.Nack(false, false)
				continue
			}

			response := processBilhete(paymentResponse)

			body, err := json.Marshal(response)
			if err != nil {
				log.Printf("Erro ao codificar resposta: %s", err)
				msg.Nack(false, false) // Rejeitar mensagem sem requeue
				continue
			}

			msreservaKeyPem, err := publicKeyToPEM(msreservaPublicKey)
			if err != nil {
				log.Printf("Erro ao converter chave pública do msreserva para PEM: %v", err)
				msg.Nack(false, false) // Rejeitar mensagem sem requeue
				continue
			}

			msg.Ack(false)

			// Publicar resposta
			err = ch.Publish(
				"",
				"bilhete_gerado",
				false,
				false,
				amqp.Publishing{
					DeliveryMode: amqp.Persistent,
					ContentType:  "application/json",
					Body:         body,
					Headers:      amqp.Table{"key": msreservaKeyPem},
				})
			if err != nil {
				log.Printf("Erro ao enviar resposta: %s", err)
			} else { // Confirmar processamento bem-sucedido
				log.Printf("Resposta de reserva enviada, ID: %s, status: %s",
					response.ReservationID, response.Status)
			}
		}
	}()

	log.Printf("Microserviço de processamento de reservas iniciado. Para sair pressione CTRL+C")
	<-forever
}
