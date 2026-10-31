package main

// coordenador.go: o lado de QUEM COORDENA o itinerário (a empresa do passageiro).
// Decisão 6: 2PC. Fase 1 pergunta a todas as donas; fase 2 confirma todas ou
// cancela todas. Decisão 9: a decisão é gravada ANTES do primeiro "confirmar"
// (certidão antes do anúncio).

import (
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// reservarItinerario reserva TODOS os itens ou NENHUM.
// Devolve o ID da transação e nil se confirmou, ou um erro com o motivo.
func reservarItinerario(passageiro string, itens []protocolo.Item) (string, error) {
	partes, err := agruparPorEmpresa(itens)
	if err != nil {
		return "", err
	}

	// 1) Registra a transação como "perguntando" e grava (antes de qualquer pedido).
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

	// 2) FASE 1: pergunta a todas as donas AO MESMO TEMPO (uma goroutine por empresa).
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
	talvezCair("apos-fase1") // demonstração: cair sem decisão gravada

	// 3) Decide e GRAVA a decisão (a certidão).
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
	// Se alguém perguntou "e aí?" enquanto esperávamos, a transação já foi
	// cancelada (ajuste da decisão 8). Nesse caso NÃO vira confirmar.
	if tx.Estado == perguntando {
		if motivo == "" {
			tx.Estado = confirmar
		} else {
			tx.Estado = cancelar
			tx.Motivo = motivo
		}
	}
	if err := salvar(); err != nil {
		// Sem certidão gravada não pode confirmar ninguém.
		tx.Estado = cancelar
		tx.Motivo = "falha ao gravar a decisao"
	}
	decisao, motivoFinal := tx.Estado, tx.Motivo
	trava.Unlock()
	log.Printf("[coord] %s: decisao = %s %s", tx.ID, decisao, motivoFinal)
	talvezCair("apos-decisao") // demonstração: cair com a decisão gravada, sem avisar ninguém

	// 4) FASE 2: avisa todas. Quem estiver fora do ar recebe depois (reenvio periódico).
	anunciar(tx)

	if decisao != confirmar {
		return tx.ID, errors.New(motivoFinal)
	}
	return tx.ID, nil
}

// agruparPorEmpresa separa os itens pela empresa dona de cada carona.
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

// anunciar manda a decisão a todas as partes que ainda não foram avisadas.
// Pode ser chamada várias vezes (o dono trata pedido repetido).
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

// consultarDecisao responde ao DONO que pergunta "o que foi decidido em tx?".
func consultarDecisao(id string) string {
	trava.Lock()
	defer trava.Unlock()

	tx, existe := estado.Transacoes[id]
	if !existe {
		return cancelar // sem certidão, não houve casamento
	}
	if tx.Estado == perguntando {
		// Ajuste da decisão 8: o dono cansou de esperar e a decisão ainda não
		// saiu. Cancela AGORA, para ele nunca liberar um assento que depois
		// seria confirmado (itinerário pela metade).
		tx.Estado = cancelar
		tx.Motivo = "uma empresa perguntou antes da decisao sair"
		salvar()
		log.Printf("[coord] %s: perguntaram antes da decisao -> CANCELAR", id)
	}
	return tx.Estado
}
