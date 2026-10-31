package main

// main.go: liga o servidor de UMA empresa.
//
// Configuração por variáveis de ambiente (assim a mesma imagem Docker serve
// para qualquer empresa e qualquer máquina do laboratório):
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

// variavel lê uma variável de ambiente ou devolve o padrão.
func variavel(nome, padrao string) string {
	if v := os.Getenv(nome); v != "" {
		return v
	}
	return padrao
}

// duracao lê uma variável como tempo ("30s", "5m") ou devolve o padrão.
func duracao(nome string, padrao time.Duration) time.Duration {
	if d, err := time.ParseDuration(os.Getenv(nome)); err == nil {
		return d
	}
	return padrao
}

// lerPares transforma "B=http://x:8080,C=http://y:8080" no mapa pares.
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
	delete(pares, empresa) // eu não sou meu próprio par

	if err := carregar(empresa); err != nil {
		log.Fatalf("erro ao carregar %s: %v", arquivo, err)
	}
	log.Printf("estado carregado de %s: %d carona(s), %d transacao(oes) coordenada(s), %d participacao(oes)",
		arquivo, len(estado.Caronas), len(estado.Transacoes), len(estado.Participacoes))
	log.Printf("pares: %v | provisorio %s | valvula %s | timeout %s", pares, prazoProvisorio, valvula, timeoutRede)

	recuperar()            // decisão 9: arruma o que ficou pela metade antes de atender
	go vigiarProvisorios() // decisão 8 (lado dono)
	go reenviarDecisoes()  // decisão 9 (lado coordenador)
	go ouvirClientes(variavel("PORTA_CLIENTES", "9000"))

	registrarRotas()
	porta := variavel("PORTA_API", "8080")
	log.Printf("API REST na porta %s", porta)
	log.Fatal(http.ListenAndServe(":"+porta, nil))
}
