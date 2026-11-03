package main

// Lado do dono do trecho (participante do 2PC). A precedência é a ordem de
// chegada na trava; sem assento livre a resposta é "não" na hora, sem fila,
// o que evita deadlock entre empresas.

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

// preparar é a fase 1: separa os assentos como provisórios e responde sim/não.
func preparar(tx, coordenador, passageiro string, itens []protocolo.Item) (bool, string) {
	trava.Lock()
	defer trava.Unlock()

	// Pedido repetido devolve a mesma resposta, sem separar de novo.
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
		salvar() // uma recusa não segura nada, então a falha aqui é tolerável
		return false, motivo
	}

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

// separarAssentos confere todos os trechos antes de descontar qualquer um.
// Devolve "" em caso de sucesso ou o motivo da recusa. Chamar com a trava pega.
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
	for k, n := range precisa {
		t := acharCarona(k.carona).Trechos[k.trecho]
		if t.Assentos < n {
			return fmt.Sprintf("sem assento livre em %s (%s -> %s)", k.carona, t.Origem, t.Destino)
		}
	}
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
	errJaLiberada   = errors.New("assento ja liberado (prazo maximo vencido)")
	errJaConfirmada = errors.New("transacao ja confirmada; nao pode cancelar")
)

func confirmarParticipacao(tx string) error {
	trava.Lock()
	defer trava.Unlock()

	p, existe := estado.Participacoes[tx]
	if !existe {
		return errDesconhecida
	}
	switch p.Estado {
	case confirmado:
		return nil
	case provisorio:
	default:
		// Só acontece se o assento já foi liberado pelo prazo máximo.
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
		// Volta ao provisório; o coordenador reenvia o confirmar depois.
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

func cancelarParticipacao(tx, motivo string) error {
	trava.Lock()
	defer trava.Unlock()

	p, existe := estado.Participacoes[tx]
	if !existe {
		// O cancelar pode chegar antes do preparar. Fica registrado para que
		// um preparar atrasado seja recusado.
		estado.Participacoes[tx] = &Participacao{Transacao: tx, Estado: cancelado, Desde: time.Now()}
		return salvar()
	}
	switch p.Estado {
	case cancelado, recusado:
		return nil
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
