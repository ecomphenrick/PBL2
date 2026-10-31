// Pacote protocolo: o formato das mensagens entre o CLIENTE (motorista ou
// passageiro) e o servidor da PRÓPRIA empresa, por socket TCP.
// Veio do Problema 1: cada mensagem é um JSON em uma linha, terminado em '\n'.
//
// A struct Item também é usada na API REST entre servidores.
package protocolo

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var ErrMensagemInvalida = errors.New("mensagem invalida")

// Item é um pedaço de itinerário: os trechos De..Ate-1 de UMA carona.
// Ex.: carona "B-1" com rota Feira, Conquista, Ilheus → De 0, Ate 2 = as duas pernas.
type Item struct {
	Carona string `json:"carona"` // "B-1": a letra antes do traço é a empresa dona
	De     int    `json:"de"`
	Ate    int    `json:"ate"`
}

// Opcao é um itinerário encontrado na busca (uma ou mais caronas).
type Opcao struct {
	Itens  []Item `json:"itens"`
	Resumo string `json:"resumo"`
	Preco  int    `json:"preco"`
}

// Pedido é o que o cliente manda. Só os campos da ação usada vêm preenchidos.
type Pedido struct {
	Acao    string `json:"acao"`
	Usuario string `json:"usuario,omitempty"`
	Senha   string `json:"senha,omitempty"`
	Tipo    string `json:"tipo,omitempty"` // "motorista" ou "passageiro"

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

// Resposta é o que o servidor devolve ao cliente.
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

// LerPedido lê uma linha da conexão e transforma em Pedido.
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

// LerResposta lê uma linha da conexão e transforma em Resposta.
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

// EnviarPedido escreve o Pedido como uma linha de JSON.
func EnviarPedido(destino io.Writer, p Pedido) error { return enviar(destino, p) }

// EnviarResposta escreve a Resposta como uma linha de JSON.
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
