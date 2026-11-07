package main

// Dados da empresa e gravação em disco. Cada empresa guarda só as suas caronas.

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

type Trecho struct {
	Origem      string   `json:"origem"`
	Destino     string   `json:"destino"`
	Assentos    int      `json:"assentos"` // livres; os provisórios já estão descontados
	Passageiros []string `json:"passageiros"`
}

type Carona struct {
	ID        string   `json:"id"` // prefixo = empresa dona, ex.: "A-1"
	Motorista string   `json:"motorista"`
	Data      string   `json:"data"`
	Preco     int      `json:"preco"` // por trecho
	Total     int      `json:"total"`
	Trechos   []Trecho `json:"trechos"`
}

// Estados de uma participação (lado do dono do trecho).
const (
	provisorio = "provisorio"
	confirmado = "confirmado"
	cancelado  = "cancelado"
	recusado   = "recusado"
)

// Participacao: o que o dono anota sobre cada pedido de assento.
type Participacao struct {
	Transacao   string           `json:"transacao"`
	Coordenador string           `json:"coordenador"`
	Passageiro  string           `json:"passageiro"`
	Itens       []protocolo.Item `json:"itens"`
	Estado      string           `json:"estado"`
	Desde       time.Time        `json:"desde"` // relógio local, só para medir o prazo
}

// Estados de uma transação (lado do coordenador).
const (
	perguntando = "perguntando"
	confirmar   = "confirmar"
	cancelar    = "cancelar"
)

// Parte é o pedaço do itinerário que pertence a uma empresa.
type Parte struct {
	Empresa string           `json:"empresa"`
	Itens   []protocolo.Item `json:"itens"`
	Voto    string           `json:"voto"`
	Avisada bool             `json:"avisada"` // já recebeu a decisão final
}

// Transacao: o que o coordenador anota sobre cada reserva.
type Transacao struct {
	ID         string    `json:"id"` // ex.: "A-T7"
	Passageiro string    `json:"passageiro"`
	Partes     []*Parte  `json:"partes"`
	Estado     string    `json:"estado"`
	Motivo     string    `json:"motivo,omitempty"`
	Criada     time.Time `json:"criada"`
}

type Usuario struct {
	Login string `json:"login"`
	Senha string `json:"senha"`
	Tipo  string `json:"tipo"`
}

type Estado struct {
	Empresa       string                   `json:"empresa"`
	ProxCarona    int                      `json:"prox_carona"`
	ProxTransacao int                      `json:"prox_transacao"`
	Usuarios      map[string]Usuario       `json:"usuarios"`
	Caronas       []Carona                 `json:"caronas"`
	Participacoes map[string]*Participacao `json:"participacoes"`
	Transacoes    map[string]*Transacao    `json:"transacoes"`
}

var (
	estado  Estado
	trava   sync.Mutex // protege o estado; nunca fica pega esperando a rede
	arquivo string
)

// salvar grava o estado (chamar com a trava pega). Escreve num arquivo
// temporário e depois renomeia: se cair no meio, o arquivo antigo fica inteiro.
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

// empresaDa: "B-3" -> "B".
func empresaDa(idCarona string) string {
	empresa, _, _ := strings.Cut(idCarona, "-")
	return empresa
}

// copiar faz uma cópia completa da carona, para ler fora da trava com segurança.
func copiar(c Carona) Carona {
	copia := c
	copia.Trechos = make([]Trecho, len(c.Trechos))
	for i, t := range c.Trechos {
		t.Passageiros = append([]string{}, t.Passageiros...)
		copia.Trechos[i] = t
	}
	return copia
}

// acharCarona devolve a carona para poder alterá-la (chamar com a trava pega).
func acharCarona(id string) *Carona {
	for i := range estado.Caronas {
		if estado.Caronas[i].ID == id {
			return &estado.Caronas[i]
		}
	}
	return nil
}
