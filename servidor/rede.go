package main

// Chamadas para as outras empresas. Toda chamada tem timeout: sem resposta,
// a empresa é tratada como fora do ar e o resto continua funcionando.
// Se o destino é a própria empresa, chama a função local direto.

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
	pares       = map[string]string{} // ex.: "B" -> "http://empresa-b:8080"
)

// chamar faz o pedido HTTP e devolve o código da resposta.
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

	r, err := clienteHTTP.Do(pedido)
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

type PedidoPreparar struct {
	Coordenador string           `json:"coordenador"`
	Passageiro  string           `json:"passageiro"`
	Itens       []protocolo.Item `json:"itens"`
}

type RespostaVoto struct {
	Voto   string `json:"voto"`
	Motivo string `json:"motivo,omitempty"`
}

type RespostaDecisao struct {
	Decisao string `json:"decisao"`
}

// pedirPreparar: sem resposta conta como "não".
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

// enviarDecisao devolve false quando precisa tentar de novo depois.
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
		return false
	}
	return codigo == http.StatusOK || codigo == http.StatusConflict
}

// perguntarDecisao: o dono pergunta ao coordenador o que foi decidido.
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
