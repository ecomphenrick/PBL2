package main

// rede.go: como ESTA empresa fala com as OUTRAS (só API REST).
// Toda chamada tem timeout (decisão 8 / regra 4): sem resposta a tempo, aquela
// empresa é tratada como fora do ar NAQUELE pedido e seguimos com o resto.
//
// Quando a "outra" empresa somos nós mesmos (passageiro usa carona da própria
// empresa), chamamos a função local direto: o caminho lógico é o mesmo.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

var (
	timeoutRede = 3 * time.Second
	clienteHTTP = &http.Client{Timeout: timeoutRede}
	pares       = map[string]string{} // empresa -> endereço base, ex.: "B" -> "http://empresa-b:8080"
)

// chamar faz um pedido HTTP a outra empresa. corpo pode ser nil; resposta
// (se não for nil) recebe o JSON devolvido. Devolve o código HTTP.
func chamar(empresa, metodo, caminho string, corpo, resposta any) (int, error) {
	base, conhecida := pares[empresa]
	if !conhecida {
		return 0, fmt.Errorf("empresa %q desconhecida", empresa)
	}
	var leitor *bytes.Reader
	if corpo != nil {
		dados, err := json.Marshal(corpo)
		if err != nil {
			return 0, err
		}
		leitor = bytes.NewReader(dados)
	} else {
		leitor = bytes.NewReader(nil)
	}
	pedido, err := http.NewRequest(metodo, base+caminho, leitor)
	if err != nil {
		return 0, err
	}
	pedido.Header.Set("Content-Type", "application/json")

	r, err := clienteHTTP.Do(pedido) // aqui o timeout age
	if err != nil {
		return 0, err
	}
	defer r.Body.Close()
	if resposta != nil {
		if err := json.NewDecoder(r.Body).Decode(resposta); err != nil {
			return r.StatusCode, err
		}
	}
	return r.StatusCode, nil
}

// Corpo do POST /transacoes/{id}/preparar.
type PedidoPreparar struct {
	Coordenador string           `json:"coordenador"`
	Passageiro  string           `json:"passageiro"`
	Itens       []protocolo.Item `json:"itens"`
}

// Resposta do preparar.
type RespostaVoto struct {
	Voto   string `json:"voto"` // "sim" ou "nao"
	Motivo string `json:"motivo,omitempty"`
}

// Resposta do GET /transacoes/{id}/decisao.
type RespostaDecisao struct {
	Decisao string `json:"decisao"` // "confirmar" ou "cancelar"
}

// pedirPreparar manda a FASE 1 a uma empresa. Sem resposta = "nao".
func pedirPreparar(empresa, tx, passageiro string, itens []protocolo.Item) (bool, string) {
	if empresa == estado.Empresa {
		return preparar(tx, estado.Empresa, passageiro, itens)
	}
	var v RespostaVoto
	corpo := PedidoPreparar{Coordenador: estado.Empresa, Passageiro: passageiro, Itens: itens}
	if _, err := chamar(empresa, "POST", "/transacoes/"+tx+"/preparar", corpo, &v); err != nil {
		return false, "empresa " + empresa + " sem resposta"
	}
	return v.Voto == "sim", v.Motivo
}

// enviarDecisao manda a FASE 2 a uma empresa. Devolve true se ela recebeu
// (ou se não adianta insistir). false = tentar de novo mais tarde.
func enviarDecisao(empresa, tx, decisao string) bool {
	if empresa == estado.Empresa {
		var err error
		if decisao == confirmar {
			err = confirmarParticipacao(tx)
		} else {
			err = cancelarParticipacao(tx, "decisao do coordenador")
		}
		return err == nil || err == errJaLiberada || err == errDesconhecida || err == errJaConfirmada
	}
	codigo, err := chamar(empresa, "POST", "/transacoes/"+tx+"/"+decisao, nil, nil)
	if err != nil {
		return false // fora do ar: o reenvio periódico tenta de novo
	}
	return codigo == http.StatusOK || codigo == http.StatusConflict
}

// perguntarDecisao: o DONO pergunta ao coordenador o que foi decidido.
func perguntarDecisao(coordenador, tx string) (string, error) {
	if coordenador == estado.Empresa {
		return consultarDecisao(tx), nil
	}
	var d RespostaDecisao
	if _, err := chamar(coordenador, "GET", "/transacoes/"+tx+"/decisao", nil, &d); err != nil {
		return "", err
	}
	return d.Decisao, nil
}
