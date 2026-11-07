package main

// Lado do dono da carona (participante do 2PC).
// Quem chega primeiro na trava leva o assento. Sem assento, responde "não"
// na hora, sem fila: assim ninguém fica esperando o outro (sem deadlock).

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

type chave struct {
	carona string
	trecho int
}

// preparar (fase 1): separa os assentos como provisórios e responde sim ou não.
func preparar(tx, coordenador, passageiro string, itens []protocolo.Item) (bool, string) {
	trava.Lock()
	defer trava.Unlock()

	// Pedido repetido: devolve a mesma resposta, sem separar de novo.
	if p, existe := estado.Participacoes[tx]; existe {
		if p.Estado == provisorio || p.Estado == confirmado {
			return true, ""
		}
		return false, "transacao " + tx + " ja foi " + p.Estado
	}

	p := &Participacao{
		Transacao:   tx,
		Coordenador: coordenador,
		Passageiro:  passageiro,
		Itens:       itens,
		Desde:       time.Now(),
	}

	if motivo := separarAssentos(itens); motivo != "" {
		p.Estado = recusado
		estado.Participacoes[tx] = p
		salvar()
		return false, motivo
	}

	// Grava antes de responder "sim".
	p.Estado = provisorio
	estado.Participacoes[tx] = p
	if err := salvar(); err != nil {
		devolverAssentos(itens)
		delete(estado.Participacoes, tx)
		return false, "falha ao gravar: " + err.Error()
	}
	log.Printf("[dono] %s: assento PROVISORIO separado (coordenador %s)", tx, coordenador)
	return true, ""
}

// separarAssentos confere todos os trechos primeiro e só depois desconta.
// Devolve "" se deu certo, ou o motivo da recusa.
func separarAssentos(itens []protocolo.Item) string {
	if len(itens) == 0 {
		return "nenhum trecho pedido"
	}
	precisa := map[chave]int{}
	for _, it := range itens {
		c := acharCarona(it.Carona)
		if c == nil {
			return "carona " + it.Carona + " nao existe"
		}
		if it.De < 0 || it.Ate > len(c.Trechos) || it.De >= it.Ate {
			return "trechos invalidos na carona " + it.Carona
		}
		for t := it.De; t < it.Ate; t++ {
			precisa[chave{it.Carona, t}]++
		}
	}
	// confere
	for k, n := range precisa {
		t := acharCarona(k.carona).Trechos[k.trecho]
		if t.Assentos < n {
			return fmt.Sprintf("sem assento livre em %s (%s -> %s)", k.carona, t.Origem, t.Destino)
		}
	}
	// desconta
	for k, n := range precisa {
		acharCarona(k.carona).Trechos[k.trecho].Assentos -= n
	}
	return ""
}

func devolverAssentos(itens []protocolo.Item) {
	for _, it := range itens {
		c := acharCarona(it.Carona)
		for t := it.De; t < it.Ate; t++ {
			c.Trechos[t].Assentos++
		}
	}
}

var (
	errDesconhecida = errors.New("transacao desconhecida")
	errJaLiberada   = errors.New("participacao ja cancelada ou recusada")
	errJaConfirmada = errors.New("transacao ja confirmada; nao pode cancelar")
)

// confirmarParticipacao (fase 2): o provisório vira confirmado.
func confirmarParticipacao(tx string) error {
	trava.Lock()
	defer trava.Unlock()

	p, existe := estado.Participacoes[tx]
	if !existe {
		return errDesconhecida
	}
	switch p.Estado {
	case confirmado:
		return nil // repetido
	case provisorio:
	default:
		log.Printf("[dono] ATENCAO %s: confirmar chegou mas a participacao esta %s", tx, p.Estado)
		return errJaLiberada
	}

	p.Estado = confirmado
	for _, it := range p.Itens {
		c := acharCarona(it.Carona)
		for t := it.De; t < it.Ate; t++ {
			c.Trechos[t].Passageiros = append(c.Trechos[t].Passageiros, p.Passageiro)
		}
	}
	if err := salvar(); err != nil {
		// Não gravou: desfaz. O coordenador manda de novo depois.
		p.Estado = provisorio
		for _, it := range p.Itens {
			c := acharCarona(it.Carona)
			for t := it.De; t < it.Ate; t++ {
				ps := c.Trechos[t].Passageiros
				c.Trechos[t].Passageiros = ps[:len(ps)-1]
			}
		}
		return err
	}
	log.Printf("[dono] %s: CONFIRMADO", tx)
	return nil
}

// cancelarParticipacao (fase 2): devolve os assentos.
func cancelarParticipacao(tx, motivo string) error {
	trava.Lock()
	defer trava.Unlock()

	p, existe := estado.Participacoes[tx]
	if !existe {
		// O "cancelar" chegou antes do "preparar". Anota, para recusar o
		// "preparar" quando ele chegar.
		estado.Participacoes[tx] = &Participacao{Transacao: tx, Estado: cancelado, Desde: time.Now()}
		return salvar()
	}
	switch p.Estado {
	case cancelado, recusado:
		return nil // repetido
	case confirmado:
		return errJaConfirmada
	}

	devolverAssentos(p.Itens)
	p.Estado = cancelado
	if err := salvar(); err != nil {
		p.Estado = provisorio
		separarAssentos(p.Itens)
		return err
	}
	log.Printf("[dono] %s: CANCELADO (%s), assentos devolvidos", tx, motivo)
	return nil
}
