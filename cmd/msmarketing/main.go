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

	"github.com/streadway/amqp"
)

var (
	privateKey           *rsa.PrivateKey
	msmarketingPublicKey *rsa.PublicKey
	cruzeiros            []model.Cruzeiro
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

	privateKeyPath := "/app/certs/msmarketing/private_key.pem"
	if _, err := os.Stat(privateKeyPath); os.IsNotExist(err) {
		privateKeyPath = "certificates/msmarketing/private_key.pem"
	}

	privateKey, err = loadPrivateKey(privateKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave privada: %v", err)
	} else {
		log.Println("Chave privada carregada com sucesso")
	}

	msmarketingPublicKeyPath := "/app/certs/msmarketing/public_key.pem"
	if _, err := os.Stat(msmarketingPublicKeyPath); os.IsNotExist(err) {
		msmarketingPublicKeyPath = "certificates/msmarketing/public_key.pem"
	}

	msmarketingPublicKey, err = loadPublicKey(msmarketingPublicKeyPath)
	if err != nil {
		log.Fatalf("Não foi possível carregar a chave pública do msmarketing: %v", err)
	} else {
		log.Println("Chave pública do msmarketing carregada com sucesso")
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

// Carrega os cruzeiros do arquivo JSON
func loadCruzeiros() {
	// Tenta abrir o arquivo JSON (considerando diferentes caminhos)
	jsonFile, err := os.Open("/app/data/cruzeiros.json")
	if err != nil {
		// Tenta caminho alternativo
		jsonFile, err = os.Open("data/cruzeiros.json")
		if err != nil {
			log.Fatalf("Não foi possível carregar o arquivo de cruzeiros: %v", err)
		}
	}
	defer jsonFile.Close()

	// Lê o conteúdo do arquivo
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Fatalf("Erro ao ler arquivo de cruzeiros: %v", err)
	}

	// Deserializa o JSON para slice de Cruzeiro
	if err := json.Unmarshal(byteValue, &cruzeiros); err != nil {
		log.Fatalf("Erro ao converter JSON de cruzeiros: %v", err)
	}

	log.Printf("Carregados %d cruzeiros com sucesso", len(cruzeiros))
}

// Envia promoções regulares a cada 5 minutos
func enviaPromocoesRegulares(ch *amqp.Channel) {
	rand.Seed(time.Now().UnixNano())
	ticker := time.NewTicker(1 * time.Minute)

	enviaPromocoes(ch)

	for range ticker.C {
		enviaPromocoes(ch)
	}
}

// Envia promoções para ambos destinos
func enviaPromocoes(ch *amqp.Channel) {
	if len(cruzeiros) == 0 {
		log.Println("Nenhum cruzeiro disponível para promoção")
		return
	}

	indice := rand.Intn(len(cruzeiros))
	cruzeiro := cruzeiros[indice]

	// Serializa o cruzeiro para JSON
	body, err := json.Marshal(cruzeiro)
	if err != nil {
		log.Printf("Erro ao serializar cruzeiro: %v", err)
		return
	}

	// Converte a chave pública para PEM
	publicKeyPEM, err := publicKeyToPEM(msmarketingPublicKey)
	if err != nil {
		log.Printf("Erro ao converter chave pública para PEM: %v", err)
		return
	}

	// Publica na exchange promocoes_destino_x
	err = ch.Publish(
		"promocoes_destino_x",
		"", // routing key vazio para fanout
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Headers: amqp.Table{
				"key": publicKeyPEM,
			},
		},
	)
	if err != nil {
		log.Printf("Erro ao publicar promoção para destino X: %v", err)
	} else {
		log.Printf("Promoção publicada para destino X: %s", cruzeiro.Nome)
	}

	// Publica na exchange promocoes_destino_y
	err = ch.Publish(
		"promocoes_destino_y",
		"", // routing key vazio para fanout
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Headers: amqp.Table{
				"key": publicKeyPEM,
			},
		},
	)
	if err != nil {
		log.Printf("Erro ao publicar promoção para destino Y: %v", err)
	} else {
		log.Printf("Promoção publicada para destino Y: %s", cruzeiro.Nome)
	}
}

func main() {
	loadCertificates()

	loadCruzeiros()

	var conn *amqp.Connection
	var err error

	for i := 0; i < 5; i++ {
		conn, err = amqp.Dial("amqp://guest:guest@rabbitmq:5672/")
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

	// Declarar as exchanges
	err = ch.ExchangeDeclare(
		"promocoes_destino_x",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar exchange de promoções X")

	err = ch.ExchangeDeclare(
		"promocoes_destino_y",
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	failOnError(err, "Falha ao declarar exchange de promoções Y")

	// Iniciar rotina de envio de promoções
	go enviaPromocoesRegulares(ch)

	log.Printf("Microserviço de marketing iniciado. Enviando promoções a cada 1 minuto...")

	// Manter o serviço em execução
	forever := make(chan bool)
	<-forever
}
