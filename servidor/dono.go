package main

// dono.go: o lado do DONO do trecho (participante do 2PC).
// Decisão 6 (2PC), 7 (precedência = ordem de chegada na trava; "ninguém espera")
// e 9 (só responde depois de gravar; idempotência pelo ID da transação).

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// chave identifica um trecho de uma carona.
type chave struct {
	carona string
	trecho int
}

// preparar é a FASE 1: "segura um assento destes trechos para a transação tx?".
// Devolve (true, "") para SIM ou (false, motivo) para NÃO.
func preparar(tx, coordenador, passageiro string, itens []protocolo.Item) (bool, string) {
	trava.Lock()
	defer trava.Unlock()

	// Pedido repetido (o coordenador reenviou): devolve a MESMA resposta,
	// sem separar assento de novo. Isso é a idempotência.
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

	// "Ninguém espera": se falta assento, responde NÃO na hora (sem fila).
	if motivo := separarAssentos(itens); motivo != "" {
		p.Estado = recusado
		estado.Participacoes[tx] = p
		salvar() // se falhar, tudo bem: um NÃO não segura nada
		return false, motivo
	}

	p.Estado = provisorio
	estado.Participacoes[tx] = p
	if err := salvar(); err != nil {
		// Não conseguiu gravar: desfaz e diz NÃO (regra de ouro do Dia 3).
		devolverAssentos(itens)
		delete(estado.Participacoes, tx)
		return false, "falha ao gravar: " + err.Error()
	}
	log.Printf("[dono] %s: assento PROVISORIO separado (coordenador %s)", tx, coordenador)
	return true, ""
}

// separarAssentos confere TODOS os trechos e só então desconta (tudo ou nada
// dentro desta empresa). Devolve "" se deu certo, ou o motivo da recusa.
// Chamar com a trava pega.
func separarAssentos(itens []protocolo.Item) string {
	if len(itens) == 0 {
		return "nenhum trecho pedido"
	}
	precisa := map[chave]int{} // quantos assentos cada trecho precisa
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

// devolverAssentos desfaz o separarAssentos. Chamar com a trava pega.
func devolverAssentos(itens []protocolo.Item) {
	for _, it := range itens {
		c := acharCarona(it.Carona)
		for t := it.De; t < it.Ate; t++ {
			c.Trechos[t].Assentos++
		}
	}
}

// Erros do confirmar que o coordenador precisa distinguir.
var (
	errDesconhecida = errors.New("transacao desconhecida")
	errJaLiberada   = errors.New("assento ja liberado (prazo maximo vencido)")
	errJaConfirmada = errors.New("transacao ja confirmada; nao pode cancelar")
)

// confirmarParticipacao é a FASE 2 quando a decisão foi CONFIRMAR.
func confirmarParticipacao(tx string) error {
	trava.Lock()
	defer trava.Unlock()

	p, existe := estado.Participacoes[tx]
	if !existe {
		return errDesconhecida
	}
	switch p.Estado {
	case confirmado:
		return nil // repetido: já estava confirmado
	case provisorio:
		// segue abaixo
	default:
		// Só acontece se a válvula de 5 min já liberou (limitação conhecida do 2PC).
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
		// Desfaz na memória; o coordenador vai reenviar o confirmar.
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

// cancelarParticipacao é a FASE 2 quando a decisão foi CANCELAR
// (e também o que a válvula de 5 min usa).
func cancelarParticipacao(tx, motivo string) error {
	trava.Lock()
	defer trava.Unlock()

	p, existe := estado.Participacoes[tx]
	if !existe {
		// O "cancelar" chegou ANTES do "preparar" (a rede pode inverter a ordem).
		// Anota como cancelada para recusar um "preparar" atrasado.
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
		separarAssentos(p.Itens) // desfaz a devolução
		return err
	}
	log.Printf("[dono] %s: CANCELADO (%s), assentos devolvidos", tx, motivo)
	return nil
}
