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

func vigiarProvisorios() {
	for range time.Tick(intervaloVigia) {
		vigiarUmaVez()
	}
}

// vigiarUmaVez resolve os provisórios vencidos. Quem votou "sim" NUNCA libera
// sozinho por prazo: o coordenador pode ter confirmado nas outras empresas.
// Primeiro pergunta ao coordenador; se ele não responde, pergunta às outras
// participantes (terminação cooperativa); se ninguém sabe, continua segurando.
func vigiarUmaVez() {
	type pendente struct {
		tx, coordenador string
		participantes   []string
	}
	var lista []pendente
	trava.Lock()
	for _, p := range estado.Participacoes {
		if p.Estado == provisorio && time.Since(p.Desde) > prazoProvisorio {
			lista = append(lista, pendente{p.Transacao, p.Coordenador, p.Participantes})
		}
	}
	trava.Unlock()

	// As perguntas são feitas fora da trava.
	for _, p := range lista {
		decisao, err := perguntarDecisao(p.coordenador, p.tx)
		if err != nil {
			decisao = perguntarParticipantes(p.tx, p.participantes)
			if decisao != "" {
				log.Printf("[dono] %s: coordenador %s fora do ar; outra participante sabia -> %s", p.tx, p.coordenador, decisao)
			}
		}
		switch decisao {
		case confirmar:
			confirmarParticipacao(p.tx)
		case "":
			log.Printf("[dono] %s: coordenador %s fora do ar e ninguem sabe a decisao; continuo segurando", p.tx, p.coordenador)
		default:
			cancelarParticipacao(p.tx, "decisao: "+decisao)
		}
	}
}

// perguntarParticipantes devolve "confirmar", "cancelar" ou "" (ninguém sabe).
//   - alguma confirmou          -> o coordenador decidiu confirmar
//   - alguma cancelou ou recusou -> o coordenador não pode ter confirmado
//   - todas provisórias/fora    -> só o coordenador sabe; esperar ele voltar
func perguntarParticipantes(tx string, participantes []string) string {
	for _, empresa := range participantes {
		if empresa == estado.Empresa {
			continue
		}
		s, err := perguntarSituacao(empresa, tx)
		if err != nil {
			continue
		}
		switch s {
		case confirmado:
			return confirmar
		case cancelado, recusado:
			return cancelar
		}
	}
	return ""
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
