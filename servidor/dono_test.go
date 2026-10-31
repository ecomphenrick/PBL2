package main

// Testes da lógica do dono e do coordenador, sem rede.
// Rodar com: go test ./servidor -v

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// prepararTeste cria uma empresa "A" vazia com uma carona A-1 (Salvador -> Feira)
// de 'assentos' lugares, gravando num diretório temporário.
func prepararTeste(t *testing.T, assentos int) {
	t.Helper()
	arquivo = filepath.Join(t.TempDir(), "estado.json")
	pares = map[string]string{}
	if err := carregar("A"); err != nil {
		t.Fatal(err)
	}
	estado.Caronas = []Carona{{
		ID: "A-1", Motorista: "rick", Data: "2026-11-03", Total: assentos,
		Trechos: []Trecho{{Origem: "Salvador", Destino: "Feira", Assentos: assentos}},
	}}
}

var umTrecho = []protocolo.Item{{Carona: "A-1", De: 0, Ate: 1}}

// Invariante 1: 100 pedidos SIMULTÂNEOS pelo último assento -> exatamente 1 "sim".
func TestUltimoAssentoSoUmGanha(t *testing.T) {
	prepararTeste(t, 1)
	var (
		espera sync.WaitGroup
		conta  sync.Mutex
		sins   int
	)
	for i := 0; i < 100; i++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			tx := "B-T" + string(rune('a'+i%26)) + string(rune('a'+i/26))
			if sim, _ := preparar(tx, "B", "p", umTrecho); sim {
				conta.Lock()
				sins++
				conta.Unlock()
			}
		}()
	}
	espera.Wait()
	if sins != 1 {
		t.Fatalf("esperava 1 sim, veio %d", sins)
	}
	if livres := estado.Caronas[0].Trechos[0].Assentos; livres != 0 {
		t.Fatalf("esperava 0 livres, veio %d", livres)
	}
}

// Pedido repetido não separa assento duas vezes (idempotência).
func TestPrepararRepetido(t *testing.T) {
	prepararTeste(t, 2)
	preparar("B-T1", "B", "p", umTrecho)
	if sim, _ := preparar("B-T1", "B", "p", umTrecho); !sim {
		t.Fatal("repetido deveria responder sim de novo")
	}
	if livres := estado.Caronas[0].Trechos[0].Assentos; livres != 1 {
		t.Fatalf("repetido separou de novo: livres = %d", livres)
	}
}

// "Cancelar" que chega ANTES do "preparar" faz o preparar atrasado ser recusado.
func TestCancelarAntesDoPreparar(t *testing.T) {
	prepararTeste(t, 1)
	cancelarParticipacao("B-T9", "teste")
	if sim, _ := preparar("B-T9", "B", "p", umTrecho); sim {
		t.Fatal("preparar atrasado deveria ser recusado")
	}
	if livres := estado.Caronas[0].Trechos[0].Assentos; livres != 1 {
		t.Fatalf("assento ficou preso: livres = %d", livres)
	}
}

// Cancelar devolve o assento; confirmar registra o passageiro.
func TestConfirmarECancelar(t *testing.T) {
	prepararTeste(t, 2)
	preparar("B-T1", "B", "ana", umTrecho)
	preparar("B-T2", "B", "bia", umTrecho)
	confirmarParticipacao("B-T1")
	cancelarParticipacao("B-T2", "teste")
	tr := estado.Caronas[0].Trechos[0]
	if tr.Assentos != 1 || len(tr.Passageiros) != 1 || tr.Passageiros[0] != "ana" {
		t.Fatalf("estado errado: %+v", tr)
	}
}

// Itinerário com a própria empresa: confirma tudo ou nada.
func TestItinerarioLocalTudoOuNada(t *testing.T) {
	prepararTeste(t, 1)
	if _, err := reservarItinerario("ana", umTrecho); err != nil {
		t.Fatalf("primeira reserva deveria confirmar: %v", err)
	}
	if _, err := reservarItinerario("bia", umTrecho); err == nil {
		t.Fatal("segunda reserva deveria ser recusada (sem assento)")
	}
}

// Ajuste da decisão 8: perguntaram a decisão enquanto ainda "perguntando" -> cancela.
func TestPerguntaAntesDaDecisaoCancela(t *testing.T) {
	prepararTeste(t, 1)
	estado.Transacoes["A-T1"] = &Transacao{ID: "A-T1", Estado: perguntando}
	if d := consultarDecisao("A-T1"); d != cancelar {
		t.Fatalf("esperava cancelar, veio %s", d)
	}
	if d := consultarDecisao("A-T999"); d != cancelar {
		t.Fatalf("transacao desconhecida deveria ser cancelar, veio %s", d)
	}
}

// Recuperação: transação sem decisão vira cancelar ao religar.
func TestRecuperarSemDecisao(t *testing.T) {
	prepararTeste(t, 1)
	estado.Transacoes["A-T1"] = &Transacao{ID: "A-T1", Estado: perguntando}
	recuperar()
	if estado.Transacoes["A-T1"].Estado != cancelar {
		t.Fatal("deveria ter virado cancelar")
	}
}

// Válvula: coordenador fora do ar por tempo demais -> o dono libera sozinho.
func TestValvulaLibera(t *testing.T) {
	prepararTeste(t, 1)
	pares = map[string]string{"B": "http://127.0.0.1:1"} // ninguém escuta aqui
	preparar("B-T1", "B", "p", umTrecho)
	estado.Participacoes["B-T1"].Desde = time.Now().Add(-10 * time.Minute)
	prazoProvisorio, valvula, intervaloVigia = time.Millisecond, time.Minute, 10*time.Millisecond
	go vigiarProvisorios()
	time.Sleep(200 * time.Millisecond)
	trava.Lock()
	defer trava.Unlock()
	if estado.Participacoes["B-T1"].Estado != cancelado || estado.Caronas[0].Trechos[0].Assentos != 1 {
		t.Fatal("valvula nao liberou o assento")
	}
}
