package main

// Quedas: o que fazer ao religar e com assentos provisórios esquecidos.

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Falha simulada (para a demonstração): derruba a empresa num ponto exato
// da próxima reserva que ela coordenar.
//
//	apos-fase1   -> depois dos votos, antes de gravar a decisão
//	apos-decisao -> depois de gravar a decisão, antes de avisar as empresas
var (
	pontoDeFalha  string
	travaDaFalha  sync.Mutex
	pontosValidos = map[string]bool{"apos-fase1": true, "apos-decisao": true}
)

func rotaSimularFalha(w http.ResponseWriter, r *http.Request) {
	ponto := r.URL.Query().Get("ponto")
	if !pontosValidos[ponto] {
		erroJSON(w, http.StatusBadRequest, "ponto deve ser apos-fase1 ou apos-decisao")
		return
	}
	travaDaFalha.Lock()
	pontoDeFalha = ponto
	travaDaFalha.Unlock()
	log.Printf("FALHA ARMADA: a proxima transacao derruba esta empresa em %q", ponto)
	responderJSON(w, http.StatusOK, map[string]string{"armada": ponto})
}

func talvezCair(ponto string) {
	travaDaFalha.Lock()
	defer travaDaFalha.Unlock()
	if pontoDeFalha == ponto {
		log.Printf("FALHA SIMULADA em %q: caindo agora!", ponto)
		os.Exit(1)
	}
}

var (
	prazoProvisorio = 30 * time.Second // depois disso, o dono pergunta ao coordenador
	intervaloVigia  = 5 * time.Second  // de quanto em quanto tempo pergunta
)

// recuperar roda ao ligar. Transação sem decisão gravada vira "cancelar":
// ninguém pode ter recebido "confirmar" antes da decisão ser gravada.
func recuperar() {
	trava.Lock()
	defer trava.Unlock()

	for _, tx := range estado.Transacoes {
		if tx.Estado == perguntando {
			tx.Estado = cancelar
			tx.Motivo = "coordenador caiu antes de decidir"
			log.Printf("[recuperacao] %s estava sem decisao -> CANCELAR", tx.ID)
		}
	}
	pendentes := 0
	for _, p := range estado.Participacoes {
		if p.Estado == provisorio {
			pendentes++
		}
	}
	if pendentes > 0 {
		log.Printf("[recuperacao] %d assento(s) provisorio(s); vou perguntar aos coordenadores", pendentes)
	}
	salvar()
}

func vigiarProvisorios() {
	for range time.Tick(intervaloVigia) {
		vigiarUmaVez()
	}
}

// vigiarUmaVez: assento provisório antigo -> pergunta ao coordenador.
// Sem resposta, segura e pergunta de novo depois. Quem votou "sim" nunca
// libera sozinho: o coordenador pode ter confirmado nas outras empresas.
func vigiarUmaVez() {
	type pendente struct{ tx, coordenador string }
	var lista []pendente
	trava.Lock()
	for _, p := range estado.Participacoes {
		if p.Estado == provisorio && time.Since(p.Desde) > prazoProvisorio {
			lista = append(lista, pendente{p.Transacao, p.Coordenador})
		}
	}
	trava.Unlock()

	// Pergunta fora da trava, para não travar a empresa esperando a rede.
	for _, p := range lista {
		decisao, err := perguntarDecisao(p.coordenador, p.tx)
		switch {
		case err != nil:
			log.Printf("[dono] %s: coordenador %s nao respondeu; continuo segurando", p.tx, p.coordenador)
		case decisao == confirmar:
			confirmarParticipacao(p.tx)
		default:
			cancelarParticipacao(p.tx, "coordenador respondeu "+decisao)
		}
	}
}

// reenviarDecisoes manda de novo a decisão para quem ainda não recebeu.
func reenviarDecisoes() {
	for range time.Tick(intervaloVigia) {
		var lista []*Transacao
		trava.Lock()
		for _, tx := range estado.Transacoes {
			if tx.Estado == perguntando {
				continue
			}
			for _, p := range tx.Partes {
				if !p.Avisada {
					lista = append(lista, tx)
					break
				}
			}
		}
		trava.Unlock()

		for _, tx := range lista {
			anunciar(tx)
		}
	}
}
