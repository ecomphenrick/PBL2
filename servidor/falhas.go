package main

// falhas.go: o que acontece quando alguém cai (decisões 8 e 9).
//
//  - recuperar: ao religar, transação sem decisão gravada vira "cancelar".
//  - vigiarProvisorios (DONO): provisório com prazo vencido pergunta ao
//    coordenador; sem resposta, segura e pergunta de novo; passou da
//    válvula (5 min), libera sozinho.
//  - reenviarDecisoes (COORDENADOR): quem não recebeu a decisão recebe de novo.

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// ---------- Falha simulada (só para a demonstração e o teste) ----------
//
// POST /simular-falha?ponto=apos-fase1   cai depois dos votos, ANTES de gravar a decisão
// POST /simular-falha?ponto=apos-decisao cai DEPOIS de gravar a decisão, antes de avisar
//
// Vale para a PRÓXIMA transação coordenada por esta empresa. O processo morre
// de verdade (como se o contêiner tivesse caído naquele instante).

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

// talvezCair derruba o processo se a falha estiver armada neste ponto.
func talvezCair(ponto string) {
	travaDaFalha.Lock()
	defer travaDaFalha.Unlock()
	if pontoDeFalha == ponto {
		log.Printf("FALHA SIMULADA em %q: caindo agora!", ponto)
		os.Exit(1)
	}
}

var (
	prazoProvisorio = 30 * time.Second // depois disso o dono pergunta ao coordenador
	valvula         = 5 * time.Minute  // depois disso o dono libera sozinho
	intervaloVigia  = 5 * time.Second  // de quanto em quanto tempo os vigias rodam
)

// recuperar roda UMA vez ao ligar, depois de carregar o arquivo.
func recuperar() {
	trava.Lock()
	defer trava.Unlock()

	for _, tx := range estado.Transacoes {
		if tx.Estado == perguntando {
			// Caí no meio da fase 1, sem decisão gravada: presumir cancelado.
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
		log.Printf("[recuperacao] %d assento(s) provisorio(s); o vigia vai perguntar aos coordenadores", pendentes)
	}
	salvar()
}

// vigiarProvisorios roda para sempre numa goroutine (lado DONO).
func vigiarProvisorios() {
	for range time.Tick(intervaloVigia) {
		// Copia a lista com a trava pega; pergunta pela rede SEM a trava.
		type pendente struct {
			tx, coordenador string
			desde           time.Time
		}
		var lista []pendente
		trava.Lock()
		for _, p := range estado.Participacoes {
			if p.Estado == provisorio && time.Since(p.Desde) > prazoProvisorio {
				lista = append(lista, pendente{p.Transacao, p.Coordenador, p.Desde})
			}
		}
		trava.Unlock()

		for _, p := range lista {
			decisao, err := perguntarDecisao(p.coordenador, p.tx)
			switch {
			case err == nil && decisao == confirmar:
				confirmarParticipacao(p.tx)
			case err == nil:
				cancelarParticipacao(p.tx, "coordenador respondeu "+decisao)
			case time.Since(p.desde) > valvula:
				log.Printf("[dono] %s: coordenador %s sumido ha mais de %s -> libero sozinho (valvula)",
					p.tx, p.coordenador, valvula)
				cancelarParticipacao(p.tx, "valvula de seguranca")
			default:
				log.Printf("[dono] %s: coordenador %s nao respondeu; continuo segurando", p.tx, p.coordenador)
			}
		}
	}
}

// reenviarDecisoes roda para sempre numa goroutine (lado COORDENADOR).
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
