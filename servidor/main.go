package main

// Servidor de uma empresa. A configuração vem de variáveis de ambiente, para
// a mesma imagem Docker servir a qualquer empresa e máquina:
//
//   EMPRESA          letra da empresa (A, B, C...)              padrão: A
//   PORTA_API        porta HTTP da API REST entre servidores     padrão: 8080
//   PORTA_CLIENTES   porta TCP dos clientes motorista/passageiro padrão: 9000
//   PARES            outras empresas: "B=http://IP:8080,C=http://IP:8080"
//   DADOS            arquivo onde o estado é gravado             padrão: estado-<EMPRESA>.json
//   PRAZO_PROVISORIO quanto o dono espera antes de perguntar     padrão: 30s
//   VALVULA          quanto o dono espera antes de liberar sozinho padrão: 5m

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func variavel(nome, padrao string) string {
	if v := os.Getenv(nome); v != "" {
		return v
	}
	return padrao
}

func duracao(nome string, padrao time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(nome)); err == nil {
		return d
	}
	return padrao
}

// lerPares: "B=http://x:8080,C=http://y:8080".
func lerPares(texto string) {
	for _, item := range strings.Split(texto, ",") {
		empresa, endereco, ok := strings.Cut(strings.TrimSpace(item), "=")
		if ok && empresa != "" {
			pares[empresa] = strings.TrimRight(endereco, "/")
		}
	}
}

func main() {
	empresa := variavel("EMPRESA", "A")
	log.SetPrefix("[empresa " + empresa + "] ")
	arquivo = variavel("DADOS", "estado-"+empresa+".json")
	prazoProvisorio = duracao("PRAZO_PROVISORIO", prazoProvisorio)
	valvula = duracao("VALVULA", valvula)
	lerPares(variavel("PARES", ""))
	delete(pares, empresa)

	if err := carregar(empresa); err != nil {
		log.Fatalf("erro ao carregar %s: %v", arquivo, err)
	}
	log.Printf("estado carregado de %s: %d carona(s), %d transacao(oes) coordenada(s), %d participacao(oes)",
		arquivo, len(estado.Caronas), len(estado.Transacoes), len(estado.Participacoes))
	log.Printf("pares: %v | provisorio %s | valvula %s | timeout %s", pares, prazoProvisorio, valvula, timeoutRede)

	recuperar() // antes de atender qualquer pedido
	go vigiarProvisorios()
	go reenviarDecisoes()
	go ouvirClientes(variavel("PORTA_CLIENTES", "9000"))

	registrarRotas()
	porta := variavel("PORTA_API", "8080")
	log.Printf("API REST na porta %s", porta)
	log.Fatal(http.ListenAndServe(":"+porta, nil))
}
