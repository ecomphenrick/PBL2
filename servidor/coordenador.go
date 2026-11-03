package main

// Lado do coordenador (a empresa do passageiro). Commit em duas fases: a
// decisão é gravada antes de qualquer "confirmar" ser enviado, para que ela
// possa ser reenviada se o coordenador cair no meio do anúncio.

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// reservarItinerario reserva todos os itens ou nenhum.
func reservarItinerario(passageiro string, itens []protocolo.Item) (string, error) {
	partes, err := agruparPorEmpresa(itens)
	if err != nil {
		return "", err
	}

	trava.Lock()
	estado.ProxTransacao++
	tx := &Transacao{
		ID:         fmt.Sprintf("%s-T%d", estado.Empresa, estado.ProxTransacao),
		Passageiro: passageiro,
		Partes:     partes,
		Estado:     perguntando,
		Criada:     time.Now(),
	}
	estado.Transacoes[tx.ID] = tx
	err = salvar()
	trava.Unlock()
	if err != nil {
		return "", fmt.Errorf("falha ao gravar: %v", err)
	}
	log.Printf("[coord] %s: FASE 1, perguntando a %d empresa(s)", tx.ID, len(partes))

	// Fase 1: uma goroutine por empresa.
	votos := make([]bool, len(partes))
	motivos := make([]string, len(partes))
	var espera sync.WaitGroup
	for i, parte := range partes {
		espera.Add(1)
		go func() {
			defer espera.Done()
			votos[i], motivos[i] = pedirPreparar(parte.Empresa, tx.ID, passageiro, parte.Itens)
		}()
	}
	espera.Wait()
	talvezCair("apos-fase1")

	trava.Lock()
	motivo := ""
	for i, parte := range tx.Partes {
		if votos[i] {
			parte.Voto = "sim"
		} else {
			parte.Voto = "nao"
			motivo = motivos[i]
		}
	}
	// Se um dono perguntou a decisão durante a fase 1, consultarDecisao já
	// cancelou a transação, e ela não pode mais virar confirmar.
	if tx.Estado == perguntando {
		if motivo == "" {
			tx.Estado = confirmar
		} else {
			tx.Estado = cancelar
			tx.Motivo = motivo
		}
	}
	if err := salvar(); err != nil {
		tx.Estado = cancelar
		tx.Motivo = "falha ao gravar a decisao"
	}
	decisao, motivoFinal := tx.Estado, tx.Motivo
	trava.Unlock()
	log.Printf("[coord] %s: decisao = %s %s", tx.ID, decisao, motivoFinal)
	talvezCair("apos-decisao")

	// Fase 2. Quem estiver fora do ar recebe pelo reenvio periódico.
	anunciar(tx)

	if decisao != confirmar {
		return tx.ID, errors.New(motivoFinal)
	}
	return tx.ID, nil
}

func agruparPorEmpresa(itens []protocolo.Item) ([]*Parte, error) {
	if len(itens) == 0 {
		return nil, errors.New("nenhum trecho escolhido")
	}
	var partes []*Parte
	porEmpresa := map[string]*Parte{}
	for _, it := range itens {
		empresa := empresaDa(it.Carona)
		if _, conhecida := pares[empresa]; !conhecida && empresa != estado.Empresa {
			return nil, fmt.Errorf("carona %s: empresa %q desconhecida", it.Carona, empresa)
		}
		p, existe := porEmpresa[empresa]
		if !existe {
			p = &Parte{Empresa: empresa}
			porEmpresa[empresa] = p
			partes = append(partes, p)
		}
		p.Itens = append(p.Itens, it)
	}
	return partes, nil
}

// anunciar envia a decisão às partes ainda não avisadas. Pode ser chamada
// mais de uma vez, porque o dono trata pedidos repetidos.
func anunciar(tx *Transacao) {
	trava.Lock()
	decisao := tx.Estado
	var pendentes []*Parte
	for _, p := range tx.Partes {
		if !p.Avisada {
			pendentes = append(pendentes, p)
		}
	}
	trava.Unlock()
	if decisao == perguntando || len(pendentes) == 0 {
		return
	}

	recebeu := make([]bool, len(pendentes))
	var espera sync.WaitGroup
	for i, p := range pendentes {
		espera.Add(1)
		go func() {
			defer espera.Done()
			recebeu[i] = enviarDecisao(p.Empresa, tx.ID, decisao)
		}()
	}
	espera.Wait()

	trava.Lock()
	for i, p := range pendentes {
		if recebeu[i] {
			p.Avisada = true
		} else {
			log.Printf("[coord] %s: empresa %s nao recebeu %q; tento de novo depois", tx.ID, p.Empresa, decisao)
		}
	}
	salvar()
	trava.Unlock()
}

// consultarDecisao atende o dono que pergunta o resultado de uma transação.
func consultarDecisao(id string) string {
	trava.Lock()
	defer trava.Unlock()

	tx, existe := estado.Transacoes[id]
	if !existe {
		return cancelar // sem registro, presume-se cancelada
	}
	if tx.Estado == perguntando {
		// Ainda sem decisão: cancela agora. Se respondesse "não sei", o dono
		// poderia liberar um assento que logo depois seria confirmado.
		tx.Estado = cancelar
		tx.Motivo = "uma empresa perguntou antes da decisao sair"
		salvar()
		log.Printf("[coord] %s: perguntaram antes da decisao -> CANCELAR", id)
	}
	return tx.Estado
}
