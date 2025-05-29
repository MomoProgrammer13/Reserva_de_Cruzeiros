package main

import (
	"crypto"
	cypto_rand "crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand" // Pacote para geração de números pseudo-aleatórios (para seleção)
	"os"
	"path/filepath"
	"reserva-cruzeiros/model" // Certifique-se que este caminho está correto e model.Promocao e model.Cruzeiro (se definido lá) estão definidos
	"strconv"                 // Para converter int para string (ID do cruzeiro)
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/streadway/amqp"
)

// Cruzeiro struct representa os dados de um cruzeiro do arquivo cruzeiros.json
// Idealmente, esta struct estaria definida em seu pacote model (model/model.go)
// se for compartilhada com outros microsserviços como o MSReserva.
// Por favor, certifique-se de que model.Cruzeiro esteja acessível ou defina-a aqui.
// Exemplo de definição (adapte conforme seu model/model.go ou cruzeiros.json):
type Cruzeiro struct {
	ID            int      `json:"id"`
	Nome          string   `json:"nome"`
	Empresa       string   `json:"empresa"`
	Itinerario    []string `json:"itinerario"`
	PortoEmbarque string   `json:"portoEmbarque"`
	// Adicione outros campos conforme necessário para corresponder ao cruzeiros.json
	// PortoDesembarque   string   `json:"portoDesembarque"`
	// DataEmbarque       string   `json:"dataEmbarque"`
	// DataDesembarque    string   `json:"dataDesembarque"`
	// CabinesDisponiveis int      `json:"cabinesDisponiveis"`
	// ValorCabine        float64  `json:"valorCabine"`
	// ImagemURL          string   `json:"imagemURL"`
	// DescricaoDetalhada string   `json:"descricaoDetalhada"`
}

var (
	// Chave privada deste microsserviço (MSMarketing)
	msMarketingPrivateKey *rsa.PrivateKey
	listaDeCruzeiros      []Cruzeiro // Lista para armazenar os cruzeiros carregados
)

const (
	// Prefixo para as exchanges de promoções. O nome completo será "promocoes-<destino>"
	exchangePromocoesPrefix = "promocoes"
	senderIDMSMarketing     = "msmarketing"
)

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

// loadCruzeirosData carrega os dados dos cruzeiros de um arquivo JSON.
func loadCruzeirosData(filePath string) error {
	log.Printf("Tentando carregar dados de cruzeiros de: %s", filePath)
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("falha ao ler arquivo de cruzeiros %s: %w", filePath, err)
	}
	err = json.Unmarshal(data, &listaDeCruzeiros)
	if err != nil {
		return fmt.Errorf("falha ao decodificar JSON de cruzeiros de %s: %w", filePath, err)
	}
	log.Printf("%d cruzeiros carregados de %s", len(listaDeCruzeiros), filePath)
	return nil
}

func loadPrivateKey(path string) (*rsa.PrivateKey, error) {
	// Garante que o caminho seja absoluto ou relativo ao executável
	if !filepath.IsAbs(path) {
		executableDir, errExe := os.Executable()
		if errExe == nil { // Se não conseguir o diretório do executável, tenta o path como está
			path = filepath.Join(filepath.Dir(executableDir), path)
		}
	}
	log.Printf("Tentando carregar chave privada de: %s", path)

	privateKeyData, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler chave privada de %s: %w", path, err)
	}
	block, _ := pem.Decode(privateKeyData)
	if block == nil {
		return nil, fmt.Errorf("falha ao decodificar PEM da chave privada de %s. Verifique se o arquivo é um PEM válido e contém BEGIN RSA PRIVATE KEY", path)
	}
	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// Tenta parsear como PKCS8 se PKCS1 falhar
		keyInterface, errPKCS8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if errPKCS8 != nil {
			return nil, fmt.Errorf("falha ao parsear chave privada (PKCS1: %v, PKCS8: %v) de %s", err, errPKCS8, path)
		}
		var ok bool
		privateKey, ok = keyInterface.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("chave privada de %s não é do tipo RSA após parse PKCS8", path)
		}
	}
	return privateKey, nil
}

func loadCertificates() {
	var err error
	// Carregar chave privada do MSMarketing
	privateKeyPath := os.Getenv("PRIVATE_KEY_PATH_MSMARKETING")
	if privateKeyPath == "" {
		privateKeyPath = "certificates/msmarketing/private_key.pem"
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			altPath := "/app/certs/msmarketing/private_key.pem"
			if _, errFs := os.Stat(altPath); errFs == nil {
				privateKeyPath = altPath
			} else {
				altPathRel := "certs/msmarketing/private_key.pem"
				if _, errFsRel := os.Stat(altPathRel); errFsRel == nil {
					privateKeyPath = altPathRel
				}
			}
		}
	}

	msMarketingPrivateKey, err = loadPrivateKey(privateKeyPath)
	failOnError(err, fmt.Sprintf("MSMarketing: Não foi possível carregar a chave privada de %s", privateKeyPath))
	log.Println("MSMarketing: Chave privada carregada com sucesso.")
}

// signMessage assina a mensagem usando a chave privada fornecida.
func signMessage(message []byte, privateKey *rsa.PrivateKey) ([]byte, error) {
	hashed := sha256.Sum256(message)
	signature, err := rsa.SignPKCS1v15(cypto_rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return nil, fmt.Errorf("erro ao assinar mensagem: %w", err)
	}
	return signature, nil
}

// sanitizeDestino converte um nome de destino para um formato adequado para nome de exchange/fila.
// Ex: "Mar Mediterrâneo" -> "mar-mediterraneo"
func sanitizeDestino(destino string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(destino, " ", "-"), "_", "-"))
}

func publishPromotion(ch *amqp.Channel, promocao model.Promocao) error {
	if promocao.Destino == "" {
		log.Printf("Aviso: Promoção '%s' (ID: %s) não possui um destino definido. Não será possível rotear corretamente.", promocao.NomePromocao, promocao.ID)
		return fmt.Errorf("destino da promoção é necessário para determinar a exchange")
	}

	destinoSanitizado := sanitizeDestino(promocao.Destino)
	exchangeName := fmt.Sprintf("%s-%s", exchangePromocoesPrefix, destinoSanitizado)

	err := ch.ExchangeDeclare(
		exchangeName,
		"fanout",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("falha ao declarar exchange '%s': %w", exchangeName, err)
	}
	log.Printf("Exchange '%s' (fanout) declarada/verificada.", exchangeName)

	promocao.Timestamp = time.Now()
	promocao.PublicadoPor = senderIDMSMarketing
	if promocao.ID == "" {
		promocao.ID = uuid.New().String()
	}

	body, err := json.Marshal(promocao)
	if err != nil {
		return fmt.Errorf("erro ao codificar promoção para JSON: %w", err)
	}

	signature, err := signMessage(body, msMarketingPrivateKey)
	if err != nil {
		return fmt.Errorf("erro ao assinar mensagem de promoção: %w", err)
	}

	err = ch.Publish(
		exchangeName,
		"",
		false,
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Persistent,
			ContentType:  "application/json",
			Body:         body,
			Timestamp:    time.Now(),
			Headers: amqp.Table{
				"signature": signature,
				"sender_id": senderIDMSMarketing,
			},
		})
	if err != nil {
		return fmt.Errorf("erro ao publicar promoção na exchange '%s': %w", exchangeName, err)
	}

	log.Printf("Promoção '%s' (ID: %s) para CruiseID '%s' no destino '%s' publicada com sucesso na exchange '%s'.", promocao.NomePromocao, promocao.ID, promocao.CruiseID, promocao.Destino, exchangeName)
	return nil
}

func main() {
	log.Println("Iniciando microserviço de marketing...")
	loadCertificates()

	rand.Seed(time.Now().UnixNano())

	// Carregar dados dos cruzeiros
	cruzeirosFilePath := os.Getenv("CRUZEIROS_JSON_PATH")
	if cruzeirosFilePath == "" {
		cruzeirosFilePath = "data/cruzeiros.json" // Caminho padrão relativo
		if _, statErr := os.Stat("/.dockerenv"); statErr == nil {
			// Tenta um caminho comum dentro de um container se o padrão não funcionar
			altPath := "/app/cruzeiros.json"
			if _, errFs := os.Stat(altPath); errFs == nil {
				cruzeirosFilePath = altPath
			} else {
				altPathRel := "cruzeiros.json" // Assume que está no workdir /app
				if _, errFsRel := os.Stat(altPathRel); errFsRel == nil {
					cruzeirosFilePath = altPathRel
				}
			}
		}
	}
	errLoad := loadCruzeirosData(cruzeirosFilePath)
	failOnError(errLoad, fmt.Sprintf("Falha ao carregar dados dos cruzeiros de %s. Verifique o caminho e o formato do arquivo.", cruzeirosFilePath))

	if len(listaDeCruzeiros) == 0 {
		log.Println("Nenhum cruzeiro carregado. Encerrando o MS Marketing, pois não há base para promoções.")
		return
	}

	rabbitMQURL := os.Getenv("RABBITMQ_URL")
	if rabbitMQURL == "" {
		rabbitMQURL = "amqp://guest:guest@localhost:5672/"
	}

	var conn *amqp.Connection
	var err error
	maxRetries := 10
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

	// Gerar e publicar um número de promoções aleatórias (ex: 3)
	numeroDePromocoesParaGerar := 3
	log.Printf("Gerando %d promoções aleatórias...", numeroDePromocoesParaGerar)

	for i := 0; i < numeroDePromocoesParaGerar; i++ {
		if len(listaDeCruzeiros) == 0 {
			log.Println("Não há cruzeiros disponíveis para gerar promoções.")
			break
		}

		// Seleciona um cruzeiro aleatoriamente
		indiceAleatorio := rand.Intn(len(listaDeCruzeiros))
		cruzeiroSelecionado := listaDeCruzeiros[indiceAleatorio]

		// Cria a promoção
		promocao := model.Promocao{
			ID:           uuid.New().String(),
			NomePromocao: fmt.Sprintf("Oferta Especial: %s!", cruzeiroSelecionado.Nome),
			CruiseID:     strconv.Itoa(cruzeiroSelecionado.ID), // Converte ID (int) para string
			Destino:      cruzeiroSelecionado.PortoEmbarque,    // Usa o porto de embarque como destino para roteamento
			Descricao:    fmt.Sprintf("Não perca esta chance incrível para o cruzeiro %s partindo de %s.", cruzeiroSelecionado.Nome, cruzeiroSelecionado.PortoEmbarque),
		}

		// Publica a promoção
		errPublish := publishPromotion(ch, promocao)
		if errPublish != nil {
			log.Printf("Erro ao publicar promoção '%s' para o cruzeiro '%s': %v", promocao.NomePromocao, cruzeiroSelecionado.Nome, errPublish)
		}
		time.Sleep(1 * time.Second) // Pequena pausa entre publicações
	}

	log.Printf("%d promoções aleatórias foram processadas.", numeroDePromocoesParaGerar)
	log.Println("Microserviço de marketing encerrando após publicar as promoções.")
}
