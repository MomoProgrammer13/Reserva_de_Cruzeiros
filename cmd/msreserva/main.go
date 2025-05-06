package main

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"reserva-cruzeiros/model"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/streadway/amqp"
)

var (
	privateKey           *rsa.PrivateKey
	msbilhetePublicKey   *rsa.PublicKey
	mspagamentoPublicKey *rsa.PublicKey
	pa                   amqp.Queue
	pr                   amqp.Queue
	bg                   amqp.Queue
)

var conn *amqp.Connection
var ch *amqp.Channel
var responses = make(map[string]chan interface{})

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

	// Carregar chave privada do microserviço msreserva
	privateKeyPath := "/app/certs/msreserva/private_key.pem"
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		privateKeyPath = "certificates/msreserva/private_key.pem"
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

	// Carregar chave pública do microserviço mspagamento
	mspagamentoPublicKeyPath := "/app/certs/mspagamento/public_key.pem"
	if _, err := os.Stat(mspagamentoPublicKeyPath); os.IsNotExist(err) {
		mspagamentoPublicKeyPath = "certificates/mspagamento/public_key.pem"
	}

	mspagamentoPublicKey, err = loadPublicKey(mspagamentoPublicKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave pública do mspagamento: %v", err)
	} else {
		log.Println("Chave pública do mspagamento carregada com sucesso")
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

	err = ch.ExchangeDeclare(
		"reserva_criada",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de reservas")

	pa, err = ch.QueueDeclare(
		"pagamento_aprovado",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de pagamentos aprovados")

	pr, err = ch.QueueDeclare(
		"pagamento_recusado",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de pagamentos recusados")

	bg, err = ch.QueueDeclare(
		"bilhete_gerado",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar fila de bilhete gerados")

	msgsPagamentosAprovados, err := ch.Consume(
		pa.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao registrar consumidor de pagamentos aprovados")

	var forever chan struct{}

	go func() {
		for msg := range msgsPagamentosAprovados {
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

			// Validar se a chave pública recebida corresponde à chave privada do msreserva
			if !validateKeyPair(receivedPublicKey, privateKey) {
				log.Printf("A chave pública recebida não corresponde à chave privada do msreserva")
				msg.Nack(false, true)
				continue
			}

			log.Printf("Chave pública validada com sucesso, processando pagamento aprovado")

			// Processar a mensagem
			var paymentResponse model.PaymentResponse
			if err := json.Unmarshal(msg.Body, &paymentResponse); err != nil {
				log.Printf("Erro ao decodificar mensagem de pagamento: %s", err)
				msg.Nack(false, false)
				continue
			}

			fmt.Println(paymentResponse)

			msg.Ack(false)
		}
	}()
	<-forever
}

func createReservationHandler(c echo.Context) error {

	fmt.Println("Entrou na rota de reserva")

	var request model.ReservationRequest
	if err := c.Bind(&request); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dados inválidos para reserva",
		})
	}

	// Validar dados da reserva
	if request.CruiseID == "" || request.Customer == "" || request.Passengers <= 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Dados incompletos para reserva",
		})
	}

	reservaID := uuid.New().String()

	request.ReservationID = reservaID

	// Preparar mensagem
	body, err := json.Marshal(request)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao processar requisição",
		})
	}

	mspagamentoKeyPEM, err := publicKeyToPEM(mspagamentoPublicKey)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao serializar chave pública",
		})
	}

	// Publicar mensagem
	err = ch.Publish(
		"",               // exchange
		"reserva_criada", // routing key
		false,            // mandatory
		false,            // immediate
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Headers: amqp.Table{
				"key": mspagamentoKeyPEM,
			},
		})
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Erro ao enviar mensagem",
		})
	}

	msgsPagamentosRecusados, err := ch.Consume(
		pr.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao registrar consumidor de pagamentos recusados")

	msgsBilhetesGerados, err := ch.Consume(
		bg.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao registrar consumidor de bilhetes gerados")

	var wg sync.WaitGroup

	var bilheteResponse model.BilheteResponse
	var responseReady bool = false

	fmt.Println("Entrou na goroutine de processamento")
	// Adiciona 1 ao contador do WaitGroup
	wg.Add(1)
	go func() {
		for {
			select {
			case msg := <-msgsPagamentosRecusados:
				log.Printf("Recebido pagamento recusado: %s", msg.Body)
				var ok bool
				// Verificar se há uma chave pública nos headers
				var receivedPublicKeyPEM string

				if receivedPublicKeyPEM, ok = msg.Headers["public_key"].(string); !ok {
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

				// Validar se a chave pública recebida corresponde à chave privada do msreserva
				if !validateKeyPair(receivedPublicKey, privateKey) {
					log.Printf("A chave pública recebida não corresponde à chave privada do msreserva, rejeitando mensagem")
					msg.Nack(false, false)
					continue
				}

				log.Printf("Chave pública validada com sucesso, processando pagamento recusado")

				// Processar a mensagem
				var paymentResponse model.PaymentResponse
				if err := json.Unmarshal(msg.Body, &paymentResponse); err != nil {
					log.Printf("Erro ao decodificar mensagem de pagamento: %s", err)
					msg.Nack(false, false)
					continue
				}

				fmt.Println(paymentResponse)
				msg.Ack(false)

				wg.Done()
			case msg := <-msgsBilhetesGerados:
				log.Printf("Recebido bilhete gerado: %s", msg.Body)

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

				// Validar se a chave pública recebida corresponde à chave privada do msreserva
				if !validateKeyPair(receivedPublicKey, privateKey) {
					log.Printf("A chave pública recebida não corresponde à chave privada do msreserva, rejeitando mensagem")
					msg.Nack(false, false)
					continue
				}

				log.Printf("Chave pública validada com sucesso, processando bilhete gerado")

				// Processar a mensagem
				var bilheteResponse model.BilheteResponse
				if err := json.Unmarshal(msg.Body, &bilheteResponse); err != nil {
					log.Printf("Erro ao decodificar mensagem de bilhete: %s", err)
					msg.Nack(false, false)
					continue
				}

				fmt.Println(bilheteResponse)
				responseReady = true
				msg.Ack(false)
				wg.Done()
			}
		}
	}()
	wg.Wait()
	fmt.Println("Saiu da goroutine de processamento")
	if responseReady {
		return c.JSON(http.StatusOK, bilheteResponse)
	} else {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Não foi possível gerar o bilhete",
		})
	}
}

func main() {

	loadCertificates()

	go setupRabbitMQ()
	defer conn.Close()
	defer ch.Close()

	e := echo.New()

	// Middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Rotas
	e.POST("/reservations", createReservationHandler)

	fmt.Println("Servidor de reservas iniciado na porta 8080")
	e.Logger.Fatal(e.Start(":8080"))
}
