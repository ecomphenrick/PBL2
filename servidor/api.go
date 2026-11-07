package main

// API REST entre as empresas (especificação em docs/api/API.md).
//
//   GET  /saude
//   GET  /caronas?data=AAAA-MM-DD     caronas desta empresa
//   POST /transacoes/{id}/preparar    fase 1 do 2PC   -> 200 sim | 409 nao
//   POST /transacoes/{id}/confirmar   fase 2          -> 200 | 409
//   POST /transacoes/{id}/cancelar    fase 2          -> 200 | 409
//   GET  /transacoes/{id}/decisao     dono pergunta ao coordenador
//   GET  /estado                      usado pelo verificador de invariantes
//   POST /simular-falha?ponto=...     demonstração de queda

import (
	"encoding/json"
	"net/http"
)

func registrarRotas() {
	http.HandleFunc("GET /saude", func(w http.ResponseWriter, r *http.Request) {
		responderJSON(w, http.StatusOK, map[string]string{"empresa": estado.Empresa, "status": "ok"})
	})
	http.HandleFunc("GET /caronas", listarCaronas)
	http.HandleFunc("POST /transacoes/{id}/preparar", rotaPreparar)
	http.HandleFunc("POST /transacoes/{id}/confirmar", rotaConfirmar)
	http.HandleFunc("POST /transacoes/{id}/cancelar", rotaCancelar)
	http.HandleFunc("GET /transacoes/{id}/decisao", rotaDecisao)
	http.HandleFunc("GET /estado", rotaEstado)
	http.HandleFunc("POST /simular-falha", rotaSimularFalha)
}

func responderJSON(w http.ResponseWriter, codigo int, corpo any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(codigo)
	json.NewEncoder(w).Encode(corpo)
}

func erroJSON(w http.ResponseWriter, codigo int, mensagem string) {
	responderJSON(w, codigo, map[string]string{"erro": mensagem})
}

func listarCaronas(w http.ResponseWriter, r *http.Request) {
	data := r.URL.Query().Get("data")
	trava.Lock()
	lista := []Carona{}
	for _, c := range estado.Caronas {
		if data == "" || c.Data == data {
			lista = append(lista, copiar(c))
		}
	}
	trava.Unlock()
	responderJSON(w, http.StatusOK, lista)
}

func rotaPreparar(w http.ResponseWriter, r *http.Request) {
	var p PedidoPreparar
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Coordenador == "" {
		erroJSON(w, http.StatusBadRequest, "corpo invalido")
		return
	}
	sim, motivo := preparar(r.PathValue("id"), p.Coordenador, p.Passageiro, p.Itens)
	if sim {
		responderJSON(w, http.StatusOK, RespostaVoto{Voto: "sim"})
	} else {
		responderJSON(w, http.StatusConflict, RespostaVoto{Voto: "nao", Motivo: motivo})
	}
}

func rotaConfirmar(w http.ResponseWriter, r *http.Request) {
	err := confirmarParticipacao(r.PathValue("id"))
	switch {
	case err == nil:
		responderJSON(w, http.StatusOK, map[string]string{"estado": confirmado})
	case err == errDesconhecida || err == errJaLiberada:
		erroJSON(w, http.StatusConflict, err.Error())
	default:
		erroJSON(w, http.StatusInternalServerError, err.Error()) // coordenador tenta de novo
	}
}

func rotaCancelar(w http.ResponseWriter, r *http.Request) {
	err := cancelarParticipacao(r.PathValue("id"), "decisao do coordenador")
	switch {
	case err == nil:
		responderJSON(w, http.StatusOK, map[string]string{"estado": cancelado})
	case err == errJaConfirmada:
		erroJSON(w, http.StatusConflict, err.Error())
	default:
		erroJSON(w, http.StatusInternalServerError, err.Error()) // coordenador tenta de novo
	}
}

func rotaDecisao(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, RespostaDecisao{Decisao: consultarDecisao(r.PathValue("id"))})
}

func rotaEstado(w http.ResponseWriter, r *http.Request) {
	trava.Lock()
	dados, err := json.Marshal(estado)
	trava.Unlock()
	if err != nil {
		erroJSON(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(dados)
}
