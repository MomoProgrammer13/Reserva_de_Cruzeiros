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
	msbilhetePublicKey *rsa.PublicKey
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

	// Carregar chave privada do microserviço mspagamento
	privateKeyPath := "/app/certs/mspagamento/private_key.pem"
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		privateKeyPath = "certificates/mspagamento/private_key.pem"
	}

	privateKey, err = loadPrivateKey(privateKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave privada: %v", err)
	} else {
		log.Println("Chave privada carregada com sucesso")
	}

	// Carregar chave pública do microserviço msbilhete
	msbilhetePublicKeyPath := "/app/certs/msbilhete/public_key.pem"
	if _, err := os.Stat(msbilhetePublicKeyPath); os.IsNotExist(err) {
		msbilhetePublicKeyPath = "certificates/msbilhete/public_key.pem"
	}

	msbilhetePublicKey, err = loadPublicKey(msbilhetePublicKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave pública do msbilhete: %v", err)
	} else {
		log.Println("Chave pública do msbilhete carregada com sucesso")
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

// Simulação de processamento de pagamento
func processPayment(request model.ReservationRequest) (model.PaymentResponse, bool) {
	// Em um sistema real, você integraria com um gateway de pagamento aqui

	paymentID := uuid.New().String() // Validar número do cartão (simulação simples)
	//valid := time.Now().UnixNano()%5 != 0
	valid := true
	if !valid {
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

	// Simular processamento
	time.Sleep(1 * time.Second)

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

	ch, err := conn.Channel()
	failOnError(err, "Falha ao abrir canal")

	rc, err := ch.QueueDeclare(
		"reserva_criada",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de reservas criadas")

	err = ch.ExchangeDeclare(
		"pagamento_aprovado",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de pagamento aprovado")

	err = ch.ExchangeDeclare(
		"pagamento_recusado",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de pagamento recusado")

	msgsReservasCriadas, err := ch.Consume(
		rc.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao registrar consumidor de reservas criadas")

	forever := make(chan bool)

	go func() {
		for msg := range msgsReservasCriadas {
			log.Printf("Reserva recebida: %s", msg.Body)

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

			// Validar se a chave pública recebida corresponde à chave privada do mspagamento
			if !validateKeyPair(receivedPublicKey, privateKey) {
				log.Printf("A chave pública recebida não corresponde à chave privada do mspagamento, rejeitando mensagem")
				msg.Nack(false, false)
				continue
			}

			log.Printf("Chave pública validada com sucesso, processando reserva")

			// Processar a mensagem
			var reservaRequest model.ReservationRequest
			if err := json.Unmarshal(msg.Body, &reservaRequest); err != nil {
				log.Printf("Erro ao decodificar mensagem de reserva: %s", err)
				msg.Nack(false, false)
				continue
			}

			// Processar pagamento
			response, aprovado := processPayment(reservaRequest)

			if aprovado {

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

				err = ch.Publish(
					"",                   // exchange
					"pagamento_aprovado", // routing key
					false,                // mandatory
					false,                // immediate
					amqp.Publishing{
						DeliveryMode: amqp.Persistent,
						ContentType:  "application/json",
						Body:         body,
						Headers: amqp.Table{
							"key": msreservaKeyPem,
						},
					})
				if err != nil {
					log.Printf("Erro ao enviar resposta: %s", err)
					msg.Nack(false, true) // Rejeitar mensagem com requeue
				} else {
					msg.Ack(false) // Confirmar processamento bem-sucedido
					log.Printf("Resposta de pagamento enviada para reserva: %s, status: %s",
						response.ReservationID, response.Status)
				}

				msbilheteKeyPem, err := publicKeyToPEM(msbilhetePublicKey)
				if err != nil {
					log.Printf("Erro ao converter chave pública do msbilhete para PEM: %v", err)
					msg.Nack(false, false) // Rejeitar mensagem sem requeue
					continue
				}

				err = ch.Publish(
					"",                   // exchange
					"pagamento_aprovado", // routing key
					false,                // mandatory
					false,                // immediate
					amqp.Publishing{
						DeliveryMode: amqp.Persistent,
						ContentType:  "application/json",
						Body:         body,
						Headers: amqp.Table{
							"key": msbilheteKeyPem,
						},
					})
				if err != nil {
					log.Printf("Erro ao enviar resposta: %s", err)
					msg.Nack(false, true) // Rejeitar mensagem com requeue
				} else {
					msg.Ack(false) // Confirmar processamento bem-sucedido
					log.Printf("Resposta de pagamento enviada para bilhete: %s, status: %s",
						response.ReservationID, response.Status)
				}

			} else {
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

				err = ch.Publish(
					"",                   // exchange
					"pagamento_recusado", // routing key
					false,                // mandatory
					false,                // immediate
					amqp.Publishing{
						DeliveryMode: amqp.Persistent,
						ContentType:  "application/json",
						Body:         body,
						Headers: amqp.Table{
							"key": msreservaKeyPem,
						},
					})
				if err != nil {
					log.Printf("Erro ao enviar resposta: %s", err)
					msg.Nack(false, true)
				} else {
					msg.Ack(false)
					log.Printf("Resposta de pagamento enviada para reserva: %s, status: %s",
						response.ReservationID, response.Status)
				}

			}

		}
	}()

	log.Printf("Microserviço de pagamento iniciado. Para sair pressione CTRL+C")
	<-forever
}
