package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const (
	keySize        = 2048 // Tamanho da chave RSA em bits
	certificateDir = "certificates"
)

// Lista de microserviços para os quais serão geradas chaves
var microservices = []string{
	"msreserva",
	"mspagamento",
	"msbilhete",
}

// Gera um par de chaves RSA para um microserviço específico
func generateKeysForService(serviceName string) error {
	log.Printf("Gerando chaves para o microserviço: %s", serviceName)

	// Gerar chave privada RSA
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		return fmt.Errorf("falha ao gerar chave privada: %w", err)
	}

	// Criar diretório para o microserviço se não existir
	serviceDir := filepath.Join(certificateDir, serviceName)
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		return fmt.Errorf("falha ao criar diretório %s: %w", serviceDir, err)
	}

	// Salvar chave privada
	privateKeyPath := filepath.Join(serviceDir, "private_key.pem")
	privateKeyFile, err := os.Create(privateKeyPath)
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo de chave privada: %w", err)
	}
	defer privateKeyFile.Close()

	// Codificar chave privada em formato PEM
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}
	if err := pem.Encode(privateKeyFile, privateKeyPEM); err != nil {
		return fmt.Errorf("falha ao codificar chave privada: %w", err)
	}
	log.Printf("Chave privada salva em: %s", privateKeyPath)

	// Extrair chave pública da chave privada
	publicKey := &privateKey.PublicKey

	// Salvar chave pública
	publicKeyPath := filepath.Join(serviceDir, "public_key.pem")
	publicKeyFile, err := os.Create(publicKeyPath)
	if err != nil {
		return fmt.Errorf("falha ao criar arquivo de chave pública: %w", err)
	}
	defer publicKeyFile.Close()

	// Codificar chave pública em formato PEM
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("falha ao serializar chave pública: %w", err)
	}
	publicKeyPEM := &pem.Block{
		Type:  "RSA PUBLIC KEY",
		Bytes: publicKeyBytes,
	}
	if err := pem.Encode(publicKeyFile, publicKeyPEM); err != nil {
		return fmt.Errorf("falha ao codificar chave pública: %w", err)
	}
	log.Printf("Chave pública salva em: %s", publicKeyPath)

	return nil
}

func main() {
	log.Println("Iniciando geração de chaves para microserviços...")

	// Criar diretório principal de certificados se não existir
	if err := os.MkdirAll(certificateDir, 0755); err != nil {
		log.Fatalf("Falha ao criar diretório de certificados: %v", err)
	}

	// Gerar chaves para cada microserviço
	for _, service := range microservices {
		if err := generateKeysForService(service); err != nil {
			log.Printf("Erro ao gerar chaves para %s: %v", service, err)
		} else {
			log.Printf("Chaves geradas com sucesso para %s", service)
		}
	}

	log.Println("Processo de geração de chaves concluído.")
}
