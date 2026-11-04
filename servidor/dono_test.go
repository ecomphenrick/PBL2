package main

// go test ./servidor -v

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// prepararTeste cria a empresa A com a carona A-1 (Salvador -> Feira).
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

// 100 pedidos simultâneos pelo último assento: exatamente um "sim".
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
			if sim, _ := preparar(tx, "B", "p", umTrecho, nil); sim {
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

func TestPrepararRepetido(t *testing.T) {
	prepararTeste(t, 2)
	preparar("B-T1", "B", "p", umTrecho, nil)
	if sim, _ := preparar("B-T1", "B", "p", umTrecho, nil); !sim {
		t.Fatal("repetido deveria responder sim de novo")
	}
	if livres := estado.Caronas[0].Trechos[0].Assentos; livres != 1 {
		t.Fatalf("repetido separou de novo: livres = %d", livres)
	}
}

func TestCancelarAntesDoPreparar(t *testing.T) {
	prepararTeste(t, 1)
	cancelarParticipacao("B-T9", "teste")
	if sim, _ := preparar("B-T9", "B", "p", umTrecho, nil); sim {
		t.Fatal("preparar atrasado deveria ser recusado")
	}
	if livres := estado.Caronas[0].Trechos[0].Assentos; livres != 1 {
		t.Fatalf("assento ficou preso: livres = %d", livres)
	}
}

func TestConfirmarECancelar(t *testing.T) {
	prepararTeste(t, 2)
	preparar("B-T1", "B", "ana", umTrecho, nil)
	preparar("B-T2", "B", "bia", umTrecho, nil)
	confirmarParticipacao("B-T1")
	cancelarParticipacao("B-T2", "teste")
	tr := estado.Caronas[0].Trechos[0]
	if tr.Assentos != 1 || len(tr.Passageiros) != 1 || tr.Passageiros[0] != "ana" {
		t.Fatalf("estado errado: %+v", tr)
	}
}

func TestItinerarioLocalTudoOuNada(t *testing.T) {
	prepararTeste(t, 1)
	if _, err := reservarItinerario("ana", umTrecho); err != nil {
		t.Fatalf("primeira reserva deveria confirmar: %v", err)
	}
	if _, err := reservarItinerario("bia", umTrecho); err == nil {
		t.Fatal("segunda reserva deveria ser recusada (sem assento)")
	}
}

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

func TestRecuperarSemDecisao(t *testing.T) {
	prepararTeste(t, 1)
	estado.Transacoes["A-T1"] = &Transacao{ID: "A-T1", Estado: perguntando}
	recuperar()
	if estado.Transacoes["A-T1"].Estado != cancelar {
		t.Fatal("deveria ter virado cancelar")
	}
}

// outraParticipante simula a empresa C respondendo /situacao com um valor fixo.
func outraParticipante(t *testing.T, resposta string) {
	t.Helper()
	c := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responderJSON(w, http.StatusOK, RespostaSituacao{Situacao: resposta})
	}))
	t.Cleanup(c.Close)
	pares = map[string]string{"B": "http://127.0.0.1:1", "C": c.URL} // B (coordenador) fora do ar
}

// A votou "sim" na transação de B; B sumiu e C também não sabe a decisão.
// Mesmo uma hora depois, A NÃO pode liberar o assento sozinha.
func TestVotouSimNaoExpiraSozinho(t *testing.T) {
	prepararTeste(t, 1)
	outraParticipante(t, provisorio)
	preparar("B-T1", "B", "p", umTrecho, []string{"A", "B", "C"})
	estado.Participacoes["B-T1"].Desde = time.Now().Add(-time.Hour)
	vigiarUmaVez()
	if estado.Participacoes["B-T1"].Estado != provisorio || estado.Caronas[0].Trechos[0].Assentos != 0 {
		t.Fatal("quem votou sim nao pode liberar o assento sozinho")
	}
}

// Terminação cooperativa: B sumiu, mas C já recebeu "confirmar" -> A confirma.
func TestCooperativaConfirma(t *testing.T) {
	prepararTeste(t, 1)
	outraParticipante(t, confirmado)
	preparar("B-T1", "B", "p", umTrecho, []string{"A", "B", "C"})
	estado.Participacoes["B-T1"].Desde = time.Now().Add(-time.Hour)
	vigiarUmaVez()
	if estado.Participacoes["B-T1"].Estado != confirmado {
		t.Fatal("C sabia que era confirmar; A deveria ter confirmado")
	}
}

// Terminação cooperativa: B sumiu, e C recusou -> B não pode ter confirmado.
func TestCooperativaCancela(t *testing.T) {
	prepararTeste(t, 1)
	outraParticipante(t, recusado)
	preparar("B-T1", "B", "p", umTrecho, []string{"A", "B", "C"})
	estado.Participacoes["B-T1"].Desde = time.Now().Add(-time.Hour)
	vigiarUmaVez()
	if estado.Participacoes["B-T1"].Estado != cancelado || estado.Caronas[0].Trechos[0].Assentos != 1 {
		t.Fatal("C recusou; A deveria ter cancelado e devolvido o assento")
	}
}

// Perguntaram a A por uma transação que nunca chegou: A anota "cancelado",
// e o preparar atrasado é recusado.
func TestSituacaoDesconhecidaBloqueiaPrepararAtrasado(t *testing.T) {
	prepararTeste(t, 1)
	if s := situacao("B-T5"); s != cancelado {
		t.Fatalf("esperava cancelado, veio %s", s)
	}
	if sim, _ := preparar("B-T5", "B", "p", umTrecho, nil); sim {
		t.Fatal("preparar atrasado deveria ser recusado")
	}
}
