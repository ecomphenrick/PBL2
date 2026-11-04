package main

// Dados da empresa e persistência em disco. Cada empresa guarda apenas as
// próprias caronas; nada aqui é cópia de outra empresa.

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

// Participacao é o registro do dono para cada transação que pediu assento a ele.
type Participacao struct {
	Transacao   string           `json:"transacao"`
	Coordenador string           `json:"coordenador"`
	Passageiro  string           `json:"passageiro"`
	Itens       []protocolo.Item `json:"itens"`
	Estado      string           `json:"estado"`
	Desde       time.Time        `json:"desde"` // relógio local, usado só para medir o prazo
	// Todas as empresas da transação, para perguntar a elas se o coordenador sumir.
	Participantes []string `json:"participantes,omitempty"`
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

// Transacao é o registro do coordenador; serve também como a reserva do passageiro.
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
	trava   sync.Mutex // protege todo o estado; nunca fica presa durante chamada de rede
	arquivo string
)

// salvar deve ser chamada com a trava pega. Grava num arquivo temporário e
// depois renomeia, para uma queda no meio da escrita não corromper o arquivo.
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

// copiar duplica também os trechos, para a cópia poder ser lida fora da trava
// sem disputar memória com uma reserva em andamento.
func copiar(c Carona) Carona {
	copia := c
	copia.Trechos = make([]Trecho, len(c.Trechos))
	for i, t := range c.Trechos {
		t.Passageiros = append([]string{}, t.Passageiros...)
		copia.Trechos[i] = t
	}
	return copia
}

// acharCarona devolve ponteiro para poder alterar a carona. Chamar com a trava pega.
func acharCarona(id string) *Carona {
	for i := range estado.Caronas {
		if estado.Caronas[i].ID == id {
			return &estado.Caronas[i]
		}
	}
	return nil
}
