// Mensagens entre cliente e servidor (TCP): um JSON por linha, como no Problema 1.
package protocolo

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrMensagemInvalida = errors.New("mensagem invalida")

// Item: os trechos De..Ate-1 de uma carona.
type Item struct {
	Carona string `json:"carona"`
	De     int    `json:"de"`
	Ate    int    `json:"ate"`
}

type Opcao struct {
	Itens  []Item `json:"itens"`
	Resumo string `json:"resumo"`
	Preco  int    `json:"preco"`
}

// Pedido: só os campos da ação usada vêm preenchidos.
type Pedido struct {
	Acao    string `json:"acao"`
	Usuario string `json:"usuario,omitempty"`
	Senha   string `json:"senha,omitempty"`
	Tipo    string `json:"tipo,omitempty"`

	// cadastrar carona
	Rota     []string `json:"rota,omitempty"`
	Data     string   `json:"data,omitempty"`
	Assentos int      `json:"assentos,omitempty"`
	Preco    int      `json:"preco,omitempty"`

	// buscar
	Origem  string `json:"origem,omitempty"`
	Destino string `json:"destino,omitempty"`

	// reservar
	Itens []Item `json:"itens,omitempty"`
}

type Resposta struct {
	OK       bool   `json:"ok"`
	Erro     string `json:"erro,omitempty"`
	Mensagem string `json:"mensagem,omitempty"`

	Tipo      string   `json:"tipo,omitempty"`      // login
	CaronaID  string   `json:"carona_id,omitempty"` // cadastrar
	Opcoes    []Opcao  `json:"opcoes,omitempty"`    // buscar
	Linhas    []string `json:"linhas,omitempty"`    // listagens
	Transacao string   `json:"transacao,omitempty"` // reservar
}

func LerPedido(leitor *bufio.Reader) (Pedido, error) {
	linha, err := leitor.ReadString('\n')
	if err != nil {
		return Pedido{}, err
	}
	var p Pedido
	if err := json.Unmarshal([]byte(linha), &p); err != nil {
		return Pedido{}, fmt.Errorf("%w: %v", ErrMensagemInvalida, err)
	}
	return p, nil
}

func LerResposta(leitor *bufio.Reader) (Resposta, error) {
	linha, err := leitor.ReadString('\n')
	if err != nil {
		return Resposta{}, err
	}
	var r Resposta
	if err := json.Unmarshal([]byte(linha), &r); err != nil {
		return Resposta{}, fmt.Errorf("%w: %v", ErrMensagemInvalida, err)
	}
	return r, nil
}

func EnviarPedido(destino io.Writer, p Pedido) error { return enviar(destino, p) }

func EnviarResposta(destino io.Writer, r Resposta) error { return enviar(destino, r) }

func enviar(destino io.Writer, valor any) error {
	bytes, err := json.Marshal(valor)
	if err != nil {
		return err
	}
	bytes = append(bytes, '\n')
	_, err = destino.Write(bytes)
	return err
}
