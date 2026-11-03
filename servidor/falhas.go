package main

// Tratamento de quedas: recuperação ao religar, vigia dos assentos
// provisórios (lado dono) e reenvio de decisões (lado coordenador).

import (
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Falha simulada, usada na demonstração e no teste: derruba o processo num
// ponto exato do 2PC na próxima transação coordenada por esta empresa.
//
//	apos-fase1   -> depois dos votos, antes de gravar a decisão
//	apos-decisao -> depois de gravar a decisão, antes de avisar as partes
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
	prazoProvisorio = 30 * time.Second // a partir daqui o dono pergunta ao coordenador
	valvula         = 5 * time.Minute  // a partir daqui o dono libera sem resposta
	intervaloVigia  = 5 * time.Second
)

// recuperar roda uma vez ao ligar. Transação sem decisão gravada é cancelada,
// porque nenhum "confirmar" pode ter sido enviado antes da gravação.
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
		log.Printf("[recuperacao] %d assento(s) provisorio(s); o vigia vai perguntar aos coordenadores", pendentes)
	}
	salvar()
}

// vigiarProvisorios: provisório vencido pergunta ao coordenador; sem resposta
// continua segurando, e só libera sozinho depois da válvula.
func vigiarProvisorios() {
	for range time.Tick(intervaloVigia) {
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

		// As perguntas são feitas fora da trava.
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

// reenviarDecisoes reenvia decisões que alguma parte ainda não recebeu.
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
