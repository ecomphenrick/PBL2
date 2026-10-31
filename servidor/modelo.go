package main

// modelo.go: os dados que ESTA empresa guarda e como eles vão para o disco.
// Regra do projeto: cada empresa guarda só o que é dela. Nada aqui é cópia de
// outra empresa.

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// Trecho é uma perna da carona (ex.: Salvador -> Feira). Assento é POR TRECHO.
type Trecho struct {
	Origem      string   `json:"origem"`
	Destino     string   `json:"destino"`
	Assentos    int      `json:"assentos"`    // livres agora (provisórios já descontados)
	Passageiros []string `json:"passageiros"` // confirmados neste trecho
}

// Carona publicada por um motorista desta empresa.
type Carona struct {
	ID        string   `json:"id"` // "A-1": letra da empresa + número
	Motorista string   `json:"motorista"`
	Data      string   `json:"data"`
	Preco     int      `json:"preco"` // por trecho
	Total     int      `json:"total"` // assentos do carro
	Trechos   []Trecho `json:"trechos"`
}

// Estados de uma participação (lado DONO do trecho).
const (
	provisorio = "provisorio" // assento separado, esperando a decisão do coordenador
	confirmado = "confirmado"
	cancelado  = "cancelado"
	recusado   = "recusado" // o dono disse "não" no preparar
)

// Participacao: o que o DONO lembra de cada transação que pediu assento a ele.
type Participacao struct {
	Transacao   string           `json:"transacao"`
	Coordenador string           `json:"coordenador"` // empresa que decide (a do passageiro)
	Passageiro  string           `json:"passageiro"`
	Itens       []protocolo.Item `json:"itens"`
	Estado      string           `json:"estado"`
	Desde       time.Time        `json:"desde"` // quando separou (relógio LOCAL; só mede tempo passado)
}

// Decisões de uma transação (lado COORDENADOR).
const (
	perguntando = "perguntando" // fase 1: ainda esperando os votos
	confirmar   = "confirmar"
	cancelar    = "cancelar"
)

// Parte: o pedaço do itinerário que fica com UMA empresa.
type Parte struct {
	Empresa string           `json:"empresa"`
	Itens   []protocolo.Item `json:"itens"`
	Voto    string           `json:"voto"`    // "sim", "nao" ou "" (sem resposta)
	Avisada bool             `json:"avisada"` // já recebeu a decisão final?
}

// Transacao: o que o COORDENADOR lembra de cada itinerário que coordenou.
// Funciona também como a "reserva" do passageiro.
type Transacao struct {
	ID         string    `json:"id"` // "A-T7": empresa coordenadora + número
	Passageiro string    `json:"passageiro"`
	Partes     []*Parte  `json:"partes"`
	Estado     string    `json:"estado"`
	Motivo     string    `json:"motivo,omitempty"`
	Criada     time.Time `json:"criada"`
}

// Usuario do cliente TCP desta empresa.
type Usuario struct {
	Login string `json:"login"`
	Senha string `json:"senha"`
	Tipo  string `json:"tipo"` // "motorista" ou "passageiro"
}

// Estado é tudo o que esta empresa grava em disco.
type Estado struct {
	Empresa       string                   `json:"empresa"`
	ProxCarona    int                      `json:"prox_carona"`
	ProxTransacao int                      `json:"prox_transacao"`
	Usuarios      map[string]Usuario       `json:"usuarios"`
	Caronas       []Carona                 `json:"caronas"`
	Participacoes map[string]*Participacao `json:"participacoes"` // chave: ID da transação
	Transacoes    map[string]*Transacao    `json:"transacoes"`    // chave: ID da transação
}

var (
	estado  Estado
	trava   sync.Mutex // UMA trava protege todo o estado. Nunca segurar durante chamada de rede.
	arquivo string     // onde o estado é gravado
)

// salvar grava o estado inteiro. Chamar SEMPRE com a trava pega.
// Escreve primeiro num rascunho e depois troca pelo arquivo verdadeiro: se o
// servidor cair no meio da escrita, o arquivo antigo continua inteiro.
func salvar() error {
	dados, err := json.MarshalIndent(estado, "", "  ")
	if err != nil {
		return err
	}
	rascunho := arquivo + ".tmp"
	if err := os.WriteFile(rascunho, dados, 0644); err != nil {
		return err
	}
	return os.Rename(rascunho, arquivo)
}

// carregar lê o estado ao ligar. Sem arquivo (primeira vez) começa vazio.
func carregar(empresa string) error {
	estado = Estado{
		Empresa:       empresa,
		Usuarios:      map[string]Usuario{},
		Participacoes: map[string]*Participacao{},
		Transacoes:    map[string]*Transacao{},
	}
	dados, err := os.ReadFile(arquivo)
	if errors.Is(err, os.ErrNotExist) {
		return salvar()
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(dados, &estado)
}

// empresaDa devolve a empresa dona de uma carona pelo ID ("B-3" -> "B").
func empresaDa(idCarona string) string {
	empresa, _, _ := strings.Cut(idCarona, "-")
	return empresa
}

// acharCarona devolve um ponteiro para a carona (para poder alterá-la) ou nil.
// Chamar com a trava pega.
func acharCarona(id string) *Carona {
	for i := range estado.Caronas {
		if estado.Caronas[i].ID == id {
			return &estado.Caronas[i]
		}
	}
	return nil
}
